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

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var mirrorconfiglog = logf.Log.WithName("mirrorconfig-resource")

// SetupMirrorConfigWebhookWithManager registers the webhook for MirrorConfig in the manager.
func SetupMirrorConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalk8sv1alpha1.MirrorConfig{}).
		WithValidator(&MirrorConfigCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metalk8s-scality-com-v1alpha1-mirrorconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=mirrorconfigs,verbs=create;update,versions=v1alpha1,name=vmirrorconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// MirrorConfigCustomValidator struct is responsible for validating the MirrorConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MirrorConfigCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ admission.Validator[*metalk8sv1alpha1.MirrorConfig] = &MirrorConfigCustomValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type MirrorConfig.
func (v *MirrorConfigCustomValidator) ValidateCreate(_ context.Context, mirrorconfig *metalk8sv1alpha1.MirrorConfig) (admission.Warnings, error) {
	if mirrorconfig == nil {
		return nil, fmt.Errorf("expected a MirrorConfig object but got nil")
	}
	mirrorconfiglog.Info("Validation for MirrorConfig upon creation", "name", mirrorconfig.GetName())

	return nil, validateMirrorConfig(mirrorconfig)
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type MirrorConfig.
func (v *MirrorConfigCustomValidator) ValidateUpdate(_ context.Context, _, mirrorconfig *metalk8sv1alpha1.MirrorConfig) (admission.Warnings, error) {
	if mirrorconfig == nil {
		return nil, fmt.Errorf("expected a MirrorConfig object but got nil")
	}
	mirrorconfiglog.Info("Validation for MirrorConfig upon update", "name", mirrorconfig.GetName())

	return nil, validateMirrorConfig(mirrorconfig)
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type MirrorConfig.
func (v *MirrorConfigCustomValidator) ValidateDelete(_ context.Context, mirrorconfig *metalk8sv1alpha1.MirrorConfig) (admission.Warnings, error) {
	if mirrorconfig == nil {
		return nil, fmt.Errorf("expected a MirrorConfig object but got nil")
	}
	mirrorconfiglog.Info("Validation for MirrorConfig upon deletion", "name", mirrorconfig.GetName())

	// Careful: not activated by default
	// To enable it think about changing "verbs=create;update" to "verbs=create;update;delete" in "+kubebuilder:webhook" annotation above
	return nil, nil
}

// validateMirrorConfig checks that the registry prefixes are unique within the MirrorConfig.
func validateMirrorConfig(mirrorConfig *metalk8sv1alpha1.MirrorConfig) error {
	seen := map[string]struct{}{}
	for _, mirrorRegistry := range mirrorConfig.Spec.Registries {
		if _, duplicated := seen[mirrorRegistry.Prefix]; duplicated {
			return fmt.Errorf("duplicate registry prefix %q", mirrorRegistry.Prefix)
		}
		seen[mirrorRegistry.Prefix] = struct{}{}
	}
	return nil
}
