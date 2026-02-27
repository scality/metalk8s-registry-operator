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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/hashicorp/go-version"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var solutionarchivelog = logf.Log.WithName("solutionarchive-resource")

// SetupSolutionArchiveWebhookWithManager registers the webhook for SolutionArchive in the manager.
func SetupSolutionArchiveWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalk8sv1alpha1.SolutionArchive{}).
		WithValidator(&SolutionArchiveCustomValidator{
			client: mgr.GetClient(),
		}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metalk8s-scality-com-v1alpha1-solutionarchive,mutating=false,failurePolicy=fail,sideEffects=None,groups=metalk8s.scality.com,resources=solutionarchives,verbs=create,versions=v1alpha1,name=vsolutionarchive-v1alpha1.kb.io,admissionReviewVersions=v1

// SolutionArchiveCustomValidator struct is responsible for validating the SolutionArchive resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type SolutionArchiveCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &SolutionArchiveCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	solutionarchive, ok := obj.(*metalk8sv1alpha1.SolutionArchive)
	if !ok {
		return nil, fmt.Errorf("expected a SolutionArchive object but got %T", obj)
	}
	solutionarchivelog.Info("Validation for SolutionArchive upon creation", "name", solutionarchive.GetName())

	return nil, validateSolutionArchive(ctx, v.client, solutionarchive)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// Careful: not activated by default
	// To enable it think about changing "verbs=create" to "verbs=create,update" in "+kubebuilder:webhook" annotation above
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SolutionArchive.
func (v *SolutionArchiveCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// Careful: not activated by default
	// To enable it think about changing "verbs=create" to "verbs=create,delete" in "+kubebuilder:webhook" annotation above
	return nil, nil
}

func validateSolutionArchive(ctx context.Context, c client.Client, solutionarchive *metalk8sv1alpha1.SolutionArchive) error {
	// Validate the version is conform to SemVer convention
	_, err := version.NewVersion(solutionarchive.Spec.Version)
	if err != nil {
		return fmt.Errorf("version is not conform to SemVer convention: %s", solutionarchive.Spec.Version)
	}
	solutionArchiveNameVersion := fmt.Sprintf("%s-%s", solutionarchive.Spec.Name, solutionarchive.Spec.Version)
	saList := &metalk8sv1alpha1.SolutionArchiveList{}
	if err := c.List(ctx, saList, client.MatchingFields{"SolutionArchiveNameVersion": solutionArchiveNameVersion}); err != nil {
		return err
	}
	// Check if a SolutionArchives already exists
	if len(saList.Items) > 0 {
		return fmt.Errorf("a solution archive already exists for name %s and version %s", solutionarchive.Spec.Name, solutionarchive.Spec.Version)
	}

	return nil
}
