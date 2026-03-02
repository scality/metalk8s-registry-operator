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
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	nsav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	FINALIZER_NAME      = "metalk8s.scality.com/finalizer"
	RNA_APP_LABEL_KEY   = "app.kubernetes.io/name"
	RNA_APP_LABEL_VALUE = "metalk8s-registry-node-agent"
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
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=nodesolutionarchives,verbs="*"
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=nodesolutionarchives/finalizers,verbs=update;delete
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=nodesolutionarchives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

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

	// Initialize variable to track status
	var ready bool
	nbAgentsAvailable := 0
	nbAgentReady := 0

	// 2. Add finalizer to deal with registry deletion
	if err, stop := r.handleFinalizerAndDeletion(ctx, registry); stop {
		return ctrl.Result{}, err
	}

	// 3. Change Namespace into Registry-Node-Agent manifest
	r.ChangeNamespace(ctx, *registry.Spec.Namespace)

	// 4. Reconcile the Registry Node Agent generic infrastructure resources
	if err := r.reconcileRNACoreResources(ctx, registry); err != nil {
		registry.SetAvailable(false)
		registry.SetReady(false)
		registry.Status.ReadyAgentReplicas = ptr.To(0)
		registry.SetAgentAvailable(false)
		registry.SetAgentReady(false)
		return ctrl.Result{}, err
	}

	// 5. List all nodes matching the nodeSelector
	matchingNodes := &corev1.NodeList{}
	err := r.List(ctx, matchingNodes, client.MatchingLabels(registry.Spec.NodeSelector))
	if err != nil {
		return ctrl.Result{}, err
	}
	registry.Status.SelectedNodes = make([]string, 0, len(matchingNodes.Items))
	registry.Status.Replicas = ptr.To(len(matchingNodes.Items))
	if len(matchingNodes.Items) == 0 {
		// If no matching nodes, ignore the reconcile, but update the status
		// As we watch the nodes, next time the labels will change on Nodes, it will reconcile
		log.Info("no nodes matching the nodeSelector", "nodeSelector", registry.Spec.NodeSelector)
		registry.SetAvailable(false)
		registry.SetReady(false)
		registry.Status.ReadyAgentReplicas = ptr.To(0)
		registry.SetAgentAvailable(false)
		registry.SetAgentReady(false)
		if err := r.deleteAllRegistryResources(ctx, *registry.Spec.Namespace); err != nil {
			return ctrl.Result{}, fmt.Errorf("error deleting Registry resources: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// 6. Update the status.SelectedNodes with the list of matching nodes and deploy node-specific resources
	nbAgentsAvailable, err = r.reconcilePerNodeResources(ctx, registry, matchingNodes)
	if err != nil {
		registry.SetAvailable(false)
		registry.SetReady(false)
		return ctrl.Result{}, err
	}

	// 7. Clean unused StatefulSets and associated resources
	ready, nbAgentReady, err = r.cleanupUnusedStatefulSetsAndAssociatedResources(ctx, registry)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 8. Update the status.Available
	registry.SetAvailable(true)
	registry.SetReady(ready)
	registry.SetAgentAvailable(nbAgentsAvailable == *registry.Status.Replicas)
	registry.SetAgentReady(nbAgentReady == *registry.Status.Replicas)
	registry.Status.ReadyAgentReplicas = ptr.To(nbAgentReady)

	return ctrl.Result{}, nil
}

// handleFinalizerAndDeletion handles finalizer and deletion. Returns true if reconciliation should stop.
func (r *RegistryReconciler) handleFinalizerAndDeletion(ctx context.Context, registry *metalk8sv1alpha1.Registry) (error, bool) {
	// examine DeletionTimestamp to determine if object is under deletion
	if registry.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(registry, FINALIZER_NAME) {
			controllerutil.AddFinalizer(registry, FINALIZER_NAME)
			if err := r.Update(ctx, registry); err != nil {
				return fmt.Errorf("error adding finalizer to Registry: %w", err), true
			}
		}
		return nil, false
	}

	// The object is being deleted
	if controllerutil.ContainsFinalizer(registry, FINALIZER_NAME) {
		// our finalizer is present, so lets handle any external dependency
		if err := r.deleteAllRegistryResources(ctx, *registry.Spec.Namespace); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried.
			return fmt.Errorf("error deleting Registry resources: %w", err), true
		}

		// remove our finalizer from the list and update it.
		controllerutil.RemoveFinalizer(registry, FINALIZER_NAME)
		if err := r.Update(ctx, registry); err != nil {
			return fmt.Errorf("error removing finalizer from Registry: %w", err), true
		}
	}

	// Stop reconciliation as the item is being deleted
	return nil, true
}

func (r *RegistryReconciler) reconcileRNACoreResources(ctx context.Context, registry *metalk8sv1alpha1.Registry) (err error) {
	err = r.ReconcileRNAGenericResources(ctx, registry)
	if err != nil {
		return fmt.Errorf("error reconciling Registry Node Agent generic resources: %w", err)
	}

	err = r.ReconcileRNACACertificate(ctx, *registry.Spec.Namespace, registry)
	if err != nil {
		return fmt.Errorf("error deploying Registry Node Agent CA certificate: %w", err)
	}

	err = r.ReconcileRNACAIssuer(ctx, *registry.Spec.Namespace, registry)
	if err != nil {
		return fmt.Errorf("error deploying Registry Node Agent CA issuer: %w", err)
	}

	err = r.ReconcileRNAExternalClientCACertificate(ctx, *registry.Spec.Namespace, registry)
	if err != nil {
		return fmt.Errorf("error deploying Registry Node Agent external client CA certificate: %w", err)
	}

	return nil
}

func (r *RegistryReconciler) reconcilePerNodeResources(ctx context.Context, registry *metalk8sv1alpha1.Registry, matchingNodes *corev1.NodeList) (int, error) {
	nbAgentsAvailable := 0

	for _, node := range matchingNodes.Items {
		// Determine NodeIP
		nodeIP := ""
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP {
				nodeIP = address.Address
				break
			}
		}
		// Update the field "SelectedNodes" in registry Status
		registry.Status.SelectedNodes = append(registry.Status.SelectedNodes, node.Name)
		if err := r.ReconcileRNAStatefulSet(ctx, *registry.Spec.Namespace, node.Name, registry); err != nil {
			return nbAgentsAvailable, fmt.Errorf("error deploying Registry Node Agent StatefulSet for node %s: %w", node.Name, err)
		}
		if err := r.ReconcileRNAService(ctx, *registry.Spec.Namespace, node.Name, registry); err != nil {
			return nbAgentsAvailable, fmt.Errorf("error deploying Registry Node Agent service for node %s: %w", node.Name, err)
		}
		if err := r.ReconcileRNAExternalServerCertificate(ctx, *registry.Spec.Namespace, node.Name, nodeIP, registry); err != nil {
			return nbAgentsAvailable, fmt.Errorf("error deploying Registry Node Agent external server certificate for node %s: %w", node.Name, err)
		}
		if err := r.ReconcileRNAInternalServerCertificate(ctx, *registry.Spec.Namespace, node.Name, registry); err != nil {
			return nbAgentsAvailable, fmt.Errorf("error deploying Registry Node Agent internal server certificate for node %s: %w", node.Name, err)
		}
		if err := r.ReconcileRNAClientCertificate(ctx, *registry.Spec.Namespace, node.Name, registry); err != nil {
			return nbAgentsAvailable, fmt.Errorf("error deploying Registry Node Agent client certificate for node %s: %w", node.Name, err)
		}

		nbAgentsAvailable++
	}

	return nbAgentsAvailable, nil
}

func (r *RegistryReconciler) cleanupUnusedStatefulSetsAndAssociatedResources(ctx context.Context, registry *metalk8sv1alpha1.Registry) (ready bool, nbAgentReady int, err error) {
	ready = true
	registryNodeAgentStatefulSets := &appsv1.StatefulSetList{}
	if err = r.List(ctx, registryNodeAgentStatefulSets,
		client.InNamespace(*registry.Spec.Namespace),
		client.MatchingLabels(map[string]string{RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE}),
	); err != nil {
		return false, 0, err
	}
	for _, registryNodeAgentStatefulSet := range registryNodeAgentStatefulSets.Items {
		nodeDeployed := registryNodeAgentStatefulSet.Labels["node"]
		agentReady := true
		if !slices.Contains(registry.Status.SelectedNodes, nodeDeployed) {
			if err = r.deleteUnusedResourcesByNode(ctx,
				*registry.Spec.Namespace,
				&registryNodeAgentStatefulSet,
				nodeDeployed,
			); err != nil {
				return false, 0, err
			}
		} else {
			// Retrieve the status of StatefulSet
			if registryNodeAgentStatefulSet.Status.AvailableReplicas != 1 {
				ready = false // nolint:ineffassign // ready is initialized to true
				agentReady = false
			}
		}
		if agentReady {
			nbAgentReady++
		}
	}

	return ready, nbAgentReady, nil
}

func (r *RegistryReconciler) deleteAllRegistryResources(ctx context.Context, namespace string) error {
	registryNodeAgentStatefulSets := &appsv1.StatefulSetList{}
	err := r.List(ctx, registryNodeAgentStatefulSets,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE}))
	if err != nil {
		return err
	}
	for _, registryNodeAgentStatefulSet := range registryNodeAgentStatefulSets.Items {
		nodeDeployed := registryNodeAgentStatefulSet.Labels["node"]
		err = r.deleteUnusedResourcesByNode(ctx, namespace, &registryNodeAgentStatefulSet, nodeDeployed)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RegistryReconciler) deleteUnusedResourcesByNode(ctx context.Context, namespace string, registryNodeAgentStatefulSet *appsv1.StatefulSet, nodeDeployed string) error {
	err := r.Delete(ctx, registryNodeAgentStatefulSet)
	if err != nil {
		return fmt.Errorf("error deleting Registry Node Agent StatefulSet: %w", err)
	}

	// We need to ensure for NodeSolutionArchive full deletion (including files deletion on the node)
	// before removing its finalizer
	if controllerutil.ContainsFinalizer(registryNodeAgentStatefulSet, RNA_FINALIZER_NAME) {
		// our finalizer is present, so let's check if all associated NodeSolutionArchive have been deleted
		nodeSolutionArchiveList := &nsav1alpha1.NodeSolutionArchiveList{}
		if err := r.List(ctx, nodeSolutionArchiveList, client.MatchingLabels(map[string]string{"node": nodeDeployed})); err != nil {
			return fmt.Errorf("error listing NodeSolutionArchives: %w", err)
		}
		if len(nodeSolutionArchiveList.Items) != 0 {
			return fmt.Errorf("remaining NodeSolutionArchives")
		}
		// remove our finalizer from the list and update it.
		controllerutil.RemoveFinalizer(registryNodeAgentStatefulSet, RNA_FINALIZER_NAME)
		if err := r.Update(ctx, registryNodeAgentStatefulSet); err != nil {
			return fmt.Errorf("error removing finalizer: %w", err)
		}
	}

	// Delete the Service associated to the Node
	services := &corev1.ServiceList{}
	err = r.List(ctx, services,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE, "node": nodeDeployed}),
	)
	if err != nil {
		return fmt.Errorf("error listing Registry Node Agent Services: %w", err)
	}
	for _, service := range services.Items {
		err = r.Delete(ctx, &service)
		if err != nil {
			return fmt.Errorf("error deleting Registry Node Agent Service: %w", err)
		}
	}

	// Delete the Certificates associated to the Node
	certificates := &cmv1.CertificateList{}
	err = r.List(ctx, certificates,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{RNA_APP_LABEL_KEY: RNA_APP_LABEL_VALUE, "node": nodeDeployed}),
	)
	if err != nil {
		return fmt.Errorf("error listing Registry Node Agent Certificates: %w", err)
	}
	for _, certificate := range certificates.Items {
		err = r.Delete(ctx, &certificate)
		if err != nil {
			return fmt.Errorf("error deleting Registry Node Agent Certificate: %w", err)
		}
	}

	return nil
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
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&admissionregistrationv1.ValidatingWebhookConfiguration{}).
		Owns(&cmv1.Certificate{}).
		Owns(&cmv1.Issuer{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(matchingRegistries(r.Client))).
		Named("registry").
		Complete(r)
}
