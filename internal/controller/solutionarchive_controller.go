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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nsav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SolutionArchiveReconciler reconciles a SolutionArchive object
type SolutionArchiveReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=solutionarchives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=solutionarchives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=solutionarchives/finalizers,verbs=update
// +kubebuilder:rbac:groups=metalk8s.scality.com,resources=nodesolutionarchives,verbs=get;list;watch;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SolutionArchiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconcile SolutionArchive")

	// 1. Load the SolutionArchive by name
	solutionArchive := &metalk8sv1alpha1.SolutionArchive{}
	if err := r.Get(ctx, req.NamespacedName, solutionArchive); err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure we update the status in case of early return
	original := solutionArchive.DeepCopy()
	defer func() {
		if err := r.Status().Patch(ctx, solutionArchive, client.MergeFrom(original)); err != nil {
			log.Error(err, "unable to patch SolutionArchive status")
		}
	}()

	// 2. Get the Registry object, to retrieve the Status/selectedNodes
	//    There must be exactly one Registry object
	registryList := &metalk8sv1alpha1.RegistryList{}
	if err := r.List(ctx, registryList); err != nil {
		return ctrl.Result{}, err
	}
	if len(registryList.Items) != 1 {
		log.Info("exactly one Registry object is required", "found", len(registryList.Items))
	}
	registry := registryList.Items[0]

	// 3. Generate NodeSolutionArchive objects for all nodes
	//    If no nodes are defined on registry resource, ignore
	//    It will reconcile later when registry will update its status
	if len(registry.Status.SelectedNodes) == 0 {
		log.Info("no nodes defined from the Registry")
	}

	log.V(1).Info("generating NodeSolutionArchive objects for all nodes", "nodes", registry.Status.SelectedNodes)
	for _, node := range registry.Status.SelectedNodes {
		nsaName := fmt.Sprintf("%s-%s-%s", solutionArchive.Spec.Name, solutionArchive.Spec.Version, node)
		// Kubernetes does not accept names longer than 253 characters, so we truncate the name and add a hash
		if len(nsaName) > 253 {
			nsaName = nsaName[:244] + "-" + getHash32Name(nsaName)
		}
		nodeSolutionArchive := &nsav1alpha1.NodeSolutionArchive{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsaName,
			},
		}
		nodeSolutionArchive.SetLabels(map[string]string{"node": node})

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, nodeSolutionArchive, func() error {
			nodeSolutionArchive.Spec = nsav1alpha1.NodeSolutionArchiveSpec{
				SolutionArchiveSpec: nsav1alpha1.SolutionArchiveSpec{
					Name:    solutionArchive.Spec.Name,
					Version: solutionArchive.Spec.Version,
					Validation: nsav1alpha1.SolutionArchiveValidation{
						Checksum: nsav1alpha1.SolutionArchiveChecksum{
							Type:  solutionArchive.Spec.Validation.Checksum.Type,
							Value: solutionArchive.Spec.Validation.Checksum.Value,
						},
					},
				},
				NodeName: node,
			}
			return nil
		})

		if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SolutionArchiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalk8sv1alpha1.SolutionArchive{}).
		Named("solutionarchive").
		Complete(r)
}
