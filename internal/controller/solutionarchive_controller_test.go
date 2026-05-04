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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

var _ = Describe("SolutionArchive Controller", func() {
	const registryName = "registry-sample"
	registryNamespacedName := types.NamespacedName{
		Name: registryName,
	}
	registry := &metalk8sv1alpha1.Registry{}

	BeforeEach(func() {
		By("creating a registry resource")
		err := k8sClient.Get(ctx, registryNamespacedName, registry)
		if err != nil && errors.IsNotFound(err) {
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: registryName,
				},
				Spec: metalk8sv1alpha1.RegistrySpec{
					NodeSelector: map[string]string{
						"kubernetes.io/os":                 "linux",
						"node-role.kubernetes.io/registry": "",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		}
	})

	AfterEach(func() {
		resource := &metalk8sv1alpha1.Registry{}
		err := k8sClient.Get(ctx, registryNamespacedName, resource)
		Expect(err).NotTo(HaveOccurred())

		By("Cleanup the specific resource instance Registry")
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	})

	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		solutionarchive := &metalk8sv1alpha1.SolutionArchive{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SolutionArchive")
			err := k8sClient.Get(ctx, typeNamespacedName, solutionarchive)
			if err != nil && errors.IsNotFound(err) {
				resource := &metalk8sv1alpha1.SolutionArchive{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: rnav1alpha1.SolutionArchiveSpec{
						Name:    "my-new-solution",
						Version: "1.2.0",
						Validation: &rnav1alpha1.SolutionArchiveValidation{
							Checksum: rnav1alpha1.SolutionArchiveChecksum{
								Type:  "sha256",
								Value: "123abc",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
			// Wait for the resource to be created
			time.Sleep(1 * time.Second)
		})

		AfterEach(func() {
			// Cleanup
			resource := &metalk8sv1alpha1.SolutionArchive{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SolutionArchive")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SolutionArchiveReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
