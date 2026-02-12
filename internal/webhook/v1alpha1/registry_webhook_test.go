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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Registry Webhook", func() {
	var (
		obj       *metalk8sv1alpha1.Registry
		validator RegistryCustomValidator
		defaulter RegistryCustomDefaulter
	)

	BeforeEach(func() {
		obj = &metalk8sv1alpha1.Registry{
			ObjectMeta: metav1.ObjectMeta{
				Name: "registry-with-defaults",
			},
			Spec: metalk8sv1alpha1.RegistrySpec{
				Namespace:     ptr.To("my-namespace"),
				ArchivesPath:  ptr.To("/random/path/to/archives"),
				SolutionsPath: ptr.To("/random/path/to/solutions"),
				NodeSelector: map[string]string{
					"kubernetes.io/os":                 "linux",
					"node-role.kubernetes.io/registry": "",
				},
				Server: metalk8sv1alpha1.RegistryServerSpec{
					CertificateIssuerRef: cmmetav1.ObjectReference{
						Name: "registry-server-issuer",
						Kind: "ClusterIssuer",
					},
					Image: &metalk8sv1alpha1.ImageSpec{
						Registry:   "ghcr.io/scality",
						Name:       "metalk8s-registry-server",
						Tag:        ptr.To("v1.0.0"),
						PullPolicy: ptr.To(corev1.PullIfNotPresent),
					},
				},
				Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
					CertificateIssuerRef: cmmetav1.ObjectReference{
						Name: "registry-agent-issuer",
						Kind: "ClusterIssuer",
					},
					Authentication: metalk8sv1alpha1.AuthenticationSpec{
						MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
							CASecretRef: corev1.SecretReference{
								Name:      "registry-agent-mtls-ca",
								Namespace: "my-namespace",
							},
						},
					},
					Image: &metalk8sv1alpha1.ImageSpec{
						Registry:   "ghcr.io/scality",
						Name:       "metalk8s-registry-agent",
						Tag:        ptr.To("v1.2.3"),
						PullPolicy: ptr.To(corev1.PullIfNotPresent),
					},
				},
			},
		}
		validator = RegistryCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = RegistryCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	Context("When Default is called with a wrong object type", func() {
		It("Should return an error when obj is not a Registry", func() {
			By("calling Default with a non-Registry runtime.Object")
			wrongObj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "not-a-registry"}}
			err := defaulter.Default(ctx, wrongObj)

			By("checking that an error is returned")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected an Registry object but got"))
			Expect(err.Error()).To(ContainSubstring("*v1.Pod"))
		})
	})

	Context("When creating Registry under Defaulting Webhook", func() {
		It("Should apply defaults when Spec.Namespace field is missing", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.Namespace = nil

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.Namespace).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_NAMESPACE)))
		})

		It("Should apply defaults when Spec.Namespace field is empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.Namespace = ptr.To("")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.Namespace).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_NAMESPACE)))
		})

		It("Should not apply defaults when Spec.Namespace field is not empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.Namespace = ptr.To("my-defined-namespace")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.Namespace).To(Equal(ptr.To("my-defined-namespace")))
		})
	})

	Context("When creating Registry under Defaulting Webhook", func() {
		It("Should apply defaults when Spec.ArchivesPath field is missing", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.ArchivesPath = nil

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.ArchivesPath).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_ARCHIVES_PATH)))
		})

		It("Should apply defaults when Spec.Namespace field is empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.ArchivesPath = ptr.To("")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.ArchivesPath).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_ARCHIVES_PATH)))
		})

		It("Should not apply defaults when Spec.ArchivesPath field is not empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.ArchivesPath = ptr.To("/path/to/archives")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.ArchivesPath).To(Equal(ptr.To("/path/to/archives")))
		})
	})

	Context("When creating Registry under Defaulting Webhook", func() {
		It("Should apply defaults when Spec.SolutionsPath field is missing", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.SolutionsPath = nil

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.SolutionsPath).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_SOLUTIONS_PATH)))
		})

		It("Should apply defaults when Spec.Namespace field is empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.SolutionsPath = ptr.To("")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.SolutionsPath).To(Equal(ptr.To(metalk8sv1alpha1.DEFAULT_SOLUTIONS_PATH)))
		})

		It("Should not apply defaults when Spec.ArchivesPath field is not empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.SolutionsPath = ptr.To("/path/to/solutions")

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the default values are set")
			Expect(obj.Spec.SolutionsPath).To(Equal(ptr.To("/path/to/solutions")))
		})
	})

	Context("When creating or updating Registry under Validating Webhook", func() {
		// TODO (user): Add logic for validating webhooks
		// Example:
		// It("Should deny creation if a required field is missing", func() {
		//     By("simulating an invalid creation scenario")
		//     obj.SomeRequiredField = ""
		//     Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		// })
		//
		// It("Should admit creation if all required fields are present", func() {
		//     By("simulating an invalid creation scenario")
		//     obj.SomeRequiredField = "valid_value"
		//     Expect(validator.ValidateCreate(ctx, obj)).To(BeNil())
		// })
		//
		// It("Should validate updates correctly", func() {
		//     By("simulating a valid update scenario")
		//     oldObj.SomeRequiredField = "updated_value"
		//     obj.SomeRequiredField = "updated_value"
		//     Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		// })
	})
})
