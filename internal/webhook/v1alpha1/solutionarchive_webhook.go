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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var solutionarchivelog = logf.Log.WithName("solutionarchive-resource")

// SetupSolutionArchiveWebhookWithManager registers the webhook for SolutionArchive in the manager.
func SetupSolutionArchiveWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalk8sv1alpha1.SolutionArchive{}).
		WithValidator(&SolutionArchiveCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metalk8s-scality-com-v1alpha1-solutionarchive,mutating=false,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=solutionarchives,verbs=create;update,versions=v1alpha1,name=vsolutionarchive-v1alpha1.kb.io,admissionReviewVersions=v1

// SolutionArchiveCustomValidator struct is responsible for validating the SolutionArchive resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type SolutionArchiveCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &SolutionArchiveCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	solutionarchive, ok := obj.(*metalk8sv1alpha1.SolutionArchive)
	if !ok {
		return nil, fmt.Errorf("expected a SolutionArchive object but got %T", obj)
	}
	solutionarchivelog.Info("Validation for SolutionArchive upon creation", "name", solutionarchive.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	solutionarchive, ok := newObj.(*metalk8sv1alpha1.SolutionArchive)
	if !ok {
		return nil, fmt.Errorf("expected a SolutionArchive object for the newObj but got %T", newObj)
	}
	solutionarchivelog.Info("Validation for SolutionArchive upon update", "name", solutionarchive.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	solutionarchive, ok := obj.(*metalk8sv1alpha1.SolutionArchive)
	if !ok {
		return nil, fmt.Errorf("expected a SolutionArchive object but got %T", obj)
	}
	solutionarchivelog.Info("Validation for SolutionArchive upon deletion", "name", solutionarchive.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
