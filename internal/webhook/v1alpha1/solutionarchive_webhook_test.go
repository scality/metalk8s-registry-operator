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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SolutionArchive Webhook", func() {
	var (
		obj       *metalk8sv1alpha1.SolutionArchive
		validator SolutionArchiveCustomValidator
	)

	BeforeEach(func() {
		obj = &metalk8sv1alpha1.SolutionArchive{
			ObjectMeta: metav1.ObjectMeta{Name: "test-solutionarchive"},
			Spec: rnav1alpha1.SolutionArchiveSpec{
				Name:    "test-solution",
				Version: "1.0.0",
				Validation: rnav1alpha1.SolutionArchiveValidation{
					Checksum: rnav1alpha1.SolutionArchiveChecksum{
						Type:  "sha256",
						Value: "123abc",
					},
				},
			},
		}
		validator = SolutionArchiveCustomValidator{client: k8sClient}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When ValidateCreate is called with a wrong object type", func() {
		It("Should return an error when obj is not a SolutionArchive", func() {
			By("calling ValidateCreate with a non-SolutionArchive runtime.Object")
			wrongObj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "not-a-solutionarchive"}}
			_, err := validator.ValidateCreate(ctx, wrongObj)

			By("checking that an error is returned")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a SolutionArchive object but got"))
			Expect(err.Error()).To(ContainSubstring("*v1.Pod"))
		})
	})

	Context("When creating SolutionArchive under Validating Webhook", func() {
		It("Should deny creation if another solutionarchive already exists", func() {
			By("creating another solutionarchive")
			anotherSolutionArchive := obj.DeepCopy()
			anotherSolutionArchive.ObjectMeta = metav1.ObjectMeta{Name: "another-solutionarchive"}
			Expect(k8sClient.Create(ctx, anotherSolutionArchive)).To(Succeed())

			By("waiting for the other solutionarchive to be created")
			time.Sleep(500 * time.Millisecond)

			By("validating the creation")
			Eventually(func() error {
				_, err := validator.ValidateCreate(ctx, obj)
				return err
			}).Should(HaveOccurred())

			By("deleting the other solutionarchive to cleanup")
			Expect(k8sClient.Delete(ctx, anotherSolutionArchive)).To(Succeed())
		})
	})
})
