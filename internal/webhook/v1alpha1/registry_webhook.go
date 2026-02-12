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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var registrylog = logf.Log.WithName("registry-resource")

// SetupRegistryWebhookWithManager registers the webhook for Registry in the manager.
func SetupRegistryWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalk8sv1alpha1.Registry{}).
		WithValidator(&RegistryCustomValidator{}).
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
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &RegistryCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	registry, ok := obj.(*metalk8sv1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", obj)
	}
	registrylog.Info("Validation for Registry upon creation", "name", registry.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	registry, ok := newObj.(*metalk8sv1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object for the newObj but got %T", newObj)
	}
	registrylog.Info("Validation for Registry upon update", "name", registry.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Registry.
func (v *RegistryCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	registry, ok := obj.(*metalk8sv1alpha1.Registry)
	if !ok {
		return nil, fmt.Errorf("expected a Registry object but got %T", obj)
	}
	registrylog.Info("Validation for Registry upon deletion", "name", registry.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
