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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
)

const (
	MANAGED_BY_LABEL_KEY       = "app.kubernetes.io/managed-by"
	MANAGED_BY_LABEL_VALUE     = "registry-operator"
	MIRROR_ENDPOINT_KEY        = "endpoint"
	MIRROR_REGISTRIES_CONF_KEY = "registries.conf"
)

// MirrorConfigReconciler reconciles a MirrorConfig object
type MirrorConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=mirrorconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=mirrorconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=mirrorconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=registries,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile renders the per-namespace mirror ConfigMap (endpoint,
// registries.conf, ca.crt) for the MirrorConfig.
func (r *MirrorConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconcile MirrorConfig")

	mirrorConfig := &metalk8sv1alpha1.MirrorConfig{}
	if err := r.Get(ctx, req.NamespacedName, mirrorConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure we update the status in case of early return
	original := mirrorConfig.DeepCopy()
	defer func() {
		if err := r.Status().Patch(ctx, mirrorConfig, client.MergeFrom(original)); err != nil {
			log.Error(err, "unable to patch MirrorConfig status")
		}
	}()

	// Find the Registry (a single one is enforced by the Registry webhook).
	registryList := &metalk8sv1alpha1.RegistryList{}
	if err := r.List(ctx, registryList); err != nil {
		return ctrl.Result{}, err
	}
	if len(registryList.Items) == 0 {
		mirrorConfig.SetReady(false, metalk8sv1alpha1.MirrorConfigReasonRegistryNotReady, "No Registry found.")
		return ctrl.Result{}, nil
	}
	if len(registryList.Items) > 1 {
		mirrorConfig.SetReady(false, metalk8sv1alpha1.MirrorConfigReasonMultipleRegistries,
			"Multiple Registries found, refusing to pick one.")
		return ctrl.Result{}, nil
	}
	registry := &registryList.Items[0]
	registryNamespace := registry.GetRegistryNamespace()

	// Do not render until the registry is ready. An already rendered ConfigMap
	// is kept as-is when the registry degrades.
	if registry.Status.Ready == nil || !*registry.Status.Ready {
		mirrorConfig.SetReady(false, metalk8sv1alpha1.MirrorConfigReasonRegistryNotReady, "The registry is not ready.")
		return ctrl.Result{}, nil
	}

	caCrt, caSecretName := getRegistryServerCA(ctx, r.Client, registryNamespace, registry.Status.SelectedNodes)
	if caCrt == "" {
		// The registry is ready, so its server CA must exist: not finding it is
		// an error (retried with backoff).
		mirrorConfig.SetReady(false, metalk8sv1alpha1.MirrorConfigReasonRegistryNotReady, "The registry server CA is not available.")
		return ctrl.Result{}, fmt.Errorf("registry is ready but no registry server CA secret was found")
	}

	endpoint := fmt.Sprintf("%s.%s.svc:%d", RS_SERVICE_NAME, registryNamespace, RS_SERVER_PORT)
	prefixes := make([]string, 0, len(mirrorConfig.Spec.Registries))
	for _, mirrorRegistry := range mirrorConfig.Spec.Registries {
		prefixes = append(prefixes, mirrorRegistry.Prefix)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mirrorConfig.Name,
			Namespace: mirrorConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if err := controllerutil.SetControllerReference(mirrorConfig, configMap, r.Scheme); err != nil {
			return err
		}
		configMap.SetLabels(map[string]string{
			MANAGED_BY_LABEL_KEY: MANAGED_BY_LABEL_VALUE,
		})
		configMap.Data = map[string]string{
			MIRROR_ENDPOINT_KEY:        endpoint,
			MIRROR_REGISTRIES_CONF_KEY: utils.GenerateRegistriesConf(endpoint, prefixes),
			MIRROR_CA_KEY:              caCrt,
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconciling mirror ConfigMap: %w", err)
	}

	mirrorConfig.Status.CASecretRef = &corev1.SecretReference{Name: caSecretName, Namespace: registryNamespace}
	mirrorConfig.Status.ObservedRegistries = prefixes
	mirrorConfig.SetReady(true, metalk8sv1alpha1.MirrorConfigReasonConfigMapRendered, "The mirror ConfigMap has been generated.")

	return ctrl.Result{}, nil
}

// allMirrorConfigs requeues every MirrorConfig in the cluster.
func allMirrorConfigs(c client.Client) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		result := []reconcile.Request{}
		mirrorConfigList := &metalk8sv1alpha1.MirrorConfigList{}
		if err := c.List(ctx, mirrorConfigList); err != nil {
			logf.FromContext(ctx).Error(err, "unable to list MirrorConfigs")
			return result
		}
		for _, mirrorConfig := range mirrorConfigList.Items {
			result = append(result, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace},
			})
		}
		return result
	}
}

// mirrorConfigsUsingCASecret requeues the MirrorConfigs whose CA was read from
// the given Secret, per status.caSecretRef.
func (r *MirrorConfigReconciler) mirrorConfigsUsingCASecret(ctx context.Context, obj client.Object) []reconcile.Request {
	result := []reconcile.Request{}
	mirrorConfigList := &metalk8sv1alpha1.MirrorConfigList{}
	if err := r.List(ctx, mirrorConfigList); err != nil {
		logf.FromContext(ctx).Error(err, "unable to list MirrorConfigs")
		return result
	}
	for _, mirrorConfig := range mirrorConfigList.Items {
		caSecretRef := mirrorConfig.Status.CASecretRef
		if caSecretRef == nil || caSecretRef.Name != obj.GetName() || caSecretRef.Namespace != obj.GetNamespace() {
			continue
		}
		result = append(result, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace},
		})
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *MirrorConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalk8sv1alpha1.MirrorConfig{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&metalk8sv1alpha1.Registry{}, handler.EnqueueRequestsFromMapFunc(allMirrorConfigs(r.Client))).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mirrorConfigsUsingCASecret)).
		Named("mirrorconfig").
		Complete(r)
}
