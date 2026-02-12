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
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var registrylog = logf.Log.WithName("registry-resource")

// SetupRegistryWebhookWithManager registers the webhook for Registry in the manager.
func SetupRegistryWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalk8sv1alpha1.Registry{}).
		WithDefaulter(&RegistryCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-metalk8s-scality-com-v1alpha1-registry,mutating=true,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=registries,verbs=create;update,versions=v1alpha1,name=mregistry-v1alpha1.kb.io,admissionReviewVersions=v1

// RegistryCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Registry when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type RegistryCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &RegistryCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Registry.
func (d *RegistryCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	registry, ok := obj.(*metalk8sv1alpha1.Registry)

	if !ok {
		return fmt.Errorf("expected an Registry object but got %T", obj)
	}
	registrylog.Info("Defaulting for Registry", "name", registry.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}
