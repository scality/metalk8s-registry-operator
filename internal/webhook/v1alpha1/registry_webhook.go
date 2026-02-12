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

package v1alpha1

import (
	"context"
	"fmt"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const (
	CLUSTER_ISSUER_KIND = "ClusterIssuer"
	ISSUER_KIND         = "Issuer"
)

// nolint:unused
// log is for logging in this package.
var registrylog = logf.Log.WithName("registry-resource")

// SetupRegistryWebhookWithManager registers the webhook for Registry in the manager.
func SetupRegistryWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalk8sv1alpha1.Registry{}).
		WithValidator(&RegistryCustomValidator{
			client: mgr.GetClient(),
		}).
		WithDefaulter(&RegistryCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-metalk8s-scality-com-v1alpha1-registry,mutating=true,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=registries,verbs=create;update,versions=v1alpha1,name=mregistry-v1alpha1.kb.io,admissionReviewVersions=v1

// RegistryCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Registry when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type RegistryCustomDefaulter struct {
}

var _ webhook.CustomDefaulter = &RegistryCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Registry.
func (d *RegistryCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	registry, ok := obj.(*metalk8sv1alpha1.Registry)

	if !ok {
		return fmt.Errorf("expected an Registry object but got %T", obj)
	}
	registrylog.Info("Defaulting for Registry", "name", registry.GetName())

	registry.Spec.Namespace = ptr.To(registry.GetRegistryNamespace())
	registry.Spec.ArchivesPath = ptr.To(registry.GetArchivesPath())
	registry.Spec.SolutionsPath = ptr.To(registry.GetSolutionsPath())

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metalk8s-scality-com-v1alpha1-registry,mutating=false,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=registries,verbs=create;update,versions=v1alpha1,name=vregistry-v1alpha1.kb.io,admissionReviewVersions=v1

// RegistryCustomValidator struct is responsible for validating the Registry resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type RegistryCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &RegistryCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	registry, ok := obj.(*metalk8sv1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", obj)
	}
	registrylog.Info("Validation for Registry upon creation", "name", registry.GetName())

	return nil, validateRegistry(ctx, v.client, registry, false)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	registry, ok := newObj.(*metalk8sv1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", newObj)
	}
	registrylog.Info("Validation for Registry upon update", "name", registry.GetName())

	return nil, validateRegistry(ctx, v.client, registry, true)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// Careful: not activated by default
	// To enable it think about changing "verbs=create" to "verbs=create,delete" in "+kubebuilder:webhook" annotation above
	return nil, nil
}

func validateRegistry(ctx context.Context, c client.Client, registry *metalk8sv1alpha1.Registry, isUpdate bool) error {
	// On create, ensure no other registry exists. On update, the current registry is allowed to exist.
	if !isUpdate {
		registryList := &metalk8sv1alpha1.RegistryList{}
		if err := c.List(ctx, registryList); err != nil {
			return err
		}
		if len(registryList.Items) > 0 {
			return fmt.Errorf("a registry already exists")
		}
	}

	// Test that mTLS secret exists
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      registry.Spec.Agent.Authentication.MTLS.CASecretRef.Name,
		Namespace: registry.Spec.Agent.Authentication.MTLS.CASecretRef.Namespace,
	}, secret); err != nil {
		return fmt.Errorf("mTLS secretRef doesn't exist")
	}

	// Test that Registry-Server clusterIssuer/Issuer exists
	switch registry.Spec.Server.CertificateIssuerRef.Kind {
	case CLUSTER_ISSUER_KIND:
		clusterIssuer := &cmv1.ClusterIssuer{}
		if err := c.Get(ctx, types.NamespacedName{
			Name: registry.Spec.Server.CertificateIssuerRef.Name,
		}, clusterIssuer); err != nil {
			return fmt.Errorf("server clusterIssuer doesn't exist")
		}
	case ISSUER_KIND:
		issuer := &cmv1.Issuer{}
		if err := c.Get(ctx, types.NamespacedName{
			Name: registry.Spec.Server.CertificateIssuerRef.Name,
		}, issuer); err != nil {
			return fmt.Errorf("server issuer doesn't exist")
		}
	default:
		return fmt.Errorf("server.certificateIssuerRef.kind is not supported")
	}

	// Test that Registry-Node-Agent clusterIssuer/Issuer exists
	switch registry.Spec.Agent.CertificateIssuerRef.Kind {
	case CLUSTER_ISSUER_KIND:
		clusterIssuer := &cmv1.ClusterIssuer{}
		if err := c.Get(ctx, types.NamespacedName{
			Name: registry.Spec.Agent.CertificateIssuerRef.Name,
		}, clusterIssuer); err != nil {
			return fmt.Errorf("agent clusterIssuer doesn't exist")
		}
	case ISSUER_KIND:
		issuer := &cmv1.Issuer{}
		if err := c.Get(ctx, types.NamespacedName{
			Name: registry.Spec.Agent.CertificateIssuerRef.Name,
		}, issuer); err != nil {
			return fmt.Errorf("agent issuer doesn't exist")
		}
	default:
		return fmt.Errorf("agent.certificateIssuerRef.kind is not supported")
	}

	return nil
}
