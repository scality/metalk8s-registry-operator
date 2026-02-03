/*
Copyright 2025 Scality.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// RegistryReconciler reconciles a Registry object
type RegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	RNA    *utils.RegistryNodeAgent
}

// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=registries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=registries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=registries/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconcile Registry")

	// 1. Get the Registry object
	registry := &metalk8sv1alpha1.Registry{}
	if err := r.Get(ctx, req.NamespacedName, registry); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Apply defaults
	registry.WithDefaults()

	// Ensure we update the status in case of early return
	original := registry.DeepCopy()
	registry.InitStatus()
	defer func() {
		if err := r.Status().Patch(ctx, registry, client.MergeFrom(original)); err != nil {
			log.Error(err, "unable to patch Registry status")
		}
	}()

	// 2. Change Namespace into Registry-Node-Agent manifest
	r.ChangeNamespace(ctx, *registry.Spec.Namespace)

	// 3. Reconcile the Registry Node Agent generic infrastructure resources
	err := r.ReconcileRNAGenericResources(ctx, registry)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling Registry Node Agent generic resources: %w", err)
	}

	err = r.ReconcileRNACACertificate(ctx, *registry.Spec.Namespace, registry)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error deploying Registry Node Agent CA certificate: %w", err)
	}

	// 4. List all nodes matching the nodeSelector
	matchingNodes := &corev1.NodeList{}
	err = r.List(ctx, matchingNodes, client.MatchingLabels(registry.Spec.NodeSelector))
	if err != nil {
		return ctrl.Result{}, err
	}
	registry.Status.SelectedNodes = make([]string, 0, len(matchingNodes.Items))
	registry.Status.Replicas = ptr.To(len(matchingNodes.Items))
	if len(matchingNodes.Items) == 0 {
		// If no matching nodes, ignore the reconcile, but update the status
		// As we watch the nodes, next time the labels will change on Nodes, it will reconcile
		log.Info("no nodes matching the nodeSelector", "nodeSelector", registry.Spec.NodeSelector)
		return ctrl.Result{}, nil
	}

	// 5. Update the status.SelectedNodes with the list of matching nodes
	for _, node := range matchingNodes.Items {
		registry.Status.SelectedNodes = append(registry.Status.SelectedNodes, node.Name)
	}

	return ctrl.Result{}, nil
}

// matchingRegistries is a function that returns the Registry objects when event on Node matches
// their nodeSelector
func matchingRegistries(c client.Client) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		result := []reconcile.Request{}
		// List all Registry resources in the cluster
		registryList := &metalk8sv1alpha1.RegistryList{}
		if err := c.List(ctx, registryList, &client.ListOptions{}); err != nil {
			return result
		}

		// Create a reconcile.Request for every Registry object matching the nodeSelector
		// note: on label modification (including deletion): 2 events are emitted:
		// * one with old labels definition
		// * one with modified labels definition
		for _, registry := range registryList.Items {
			// If the Node (object) matches registry.Spec.NodeSelector, queue a Reconcile for the registry
			if isIncluded(registry.Spec.NodeSelector, obj.GetLabels()) {
				result = append(result, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: registry.Name,
					},
				})
			}
		}
		return result
	}
}

func isIncluded(subset, superset map[string]string) bool {
	// If the subset is larger than the superset, it can't be included
	if len(subset) > len(superset) {
		return false
	}

	for key, val := range subset {
		// Check if key exists and if the value matches
		if foundVal, exists := superset[key]; !exists || foundVal != val {
			return false
		}
	}

	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *RegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalk8sv1alpha1.Registry{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&admissionregistrationv1.ValidatingWebhookConfiguration{}).
		Owns(&cmv1.Certificate{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(matchingRegistries(r.Client))).
		Named("registry").
		Complete(r)
}
