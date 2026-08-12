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
	"k8s.io/utils/ptr"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TEST_NAMESPACE     = "my-namespace"
	MTLS_NAMESPACE     = "mtls-namespace"
	CA_SECRET_NAME     = "registry-agent-mtls-ca"
	SERVER_ISSUER_NAME = "registry-server-issuer"
	AGENT_ISSUER_NAME  = "registry-agent-issuer"
)

var _ = Describe("Registry Webhook", func() {
	var (
		obj                 *metalk8sv1alpha1.Registry
		validator           RegistryCustomValidator
		defaulter           RegistryCustomDefaulter
		caSecret            *corev1.Secret
		serverClusterIssuer *cmv1.ClusterIssuer
		agentIssuer         *cmv1.Issuer
		timeout             = 5 * time.Second
		interval            = 200 * time.Millisecond
	)

	BeforeEach(func() {
		By("creating a CA secret")
		caSecret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: CA_SECRET_NAME, Namespace: MTLS_NAMESPACE}}
		Expect(k8sClient.Create(ctx, caSecret)).To(Succeed())

		By("creating a server clusterIssuer")
		serverClusterIssuer = &cmv1.ClusterIssuer{ObjectMeta: metav1.ObjectMeta{Name: SERVER_ISSUER_NAME}}
		Expect(k8sClient.Create(ctx, serverClusterIssuer)).To(Succeed())

		By("creating a agent issuer")
		agentIssuer = &cmv1.Issuer{ObjectMeta: metav1.ObjectMeta{Name: AGENT_ISSUER_NAME, Namespace: TEST_NAMESPACE}}
		Expect(k8sClient.Create(ctx, agentIssuer)).To(Succeed())

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
						Kind: "Issuer",
					},
					Authentication: metalk8sv1alpha1.AuthenticationSpec{
						MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
							CASecretRef: corev1.SecretReference{
								Name:      "registry-agent-mtls-ca",
								Namespace: "mtls-namespace",
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
		validator = RegistryCustomValidator{client: k8sClient}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = RegistryCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		By("deleting the CA secret")
		Expect(k8sClient.Delete(ctx, caSecret)).To(Succeed())

		By("deleting the server clusterIssuer")
		Expect(k8sClient.Delete(ctx, serverClusterIssuer)).To(Succeed())

		By("deleting the agent issuer")
		Expect(k8sClient.Delete(ctx, agentIssuer)).To(Succeed())
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
		It("Should fill the partially set images with the component defaults", func() {
			By("simulating a Registry with partially set images")
			obj.Spec.Server.Image = &metalk8sv1alpha1.ImageSpec{Tag: ptr.To("custom-tag")}
			obj.Spec.Agent.Image = nil
			obj.Spec.MirrorPropagation = &metalk8sv1alpha1.MirrorPropagationSpec{
				Enabled: true,
				Image: &metalk8sv1alpha1.ImageSpec{
					PullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
				},
			}

			By("calling the Default method to apply defaults")
			defaulter.Default(ctx, obj) //nolint:errcheck // We don't care about the error here

			By("checking that the image defaults are set")
			Expect(obj.Spec.Server.Image.GetImage()).To(Equal(metalk8sv1alpha1.RegistryServerImageRegistry + "/" + metalk8sv1alpha1.RegistryServerImageName + ":custom-tag"))
			Expect(obj.Spec.Agent.Image.GetImage()).To(Equal(metalk8sv1alpha1.RegistryNodeAgentImageRegistry + "/" + metalk8sv1alpha1.RegistryNodeAgentImageName + ":" + metalk8sv1alpha1.RegistryNodeAgentImageTag))
			Expect(obj.Spec.MirrorPropagation.Image.GetImage()).To(Equal(metalk8sv1alpha1.FileReflectorImageRegistry + "/" + metalk8sv1alpha1.FileReflectorImageName + ":" + metalk8sv1alpha1.FileReflectorImageTag))
			Expect(obj.Spec.MirrorPropagation.Image.PullSecrets).To(HaveLen(1))
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

	Context("When creating Registry under Validating Webhook", func() {
		It("Should deny creation if another registry already exists", func() {
			By("creating another registry")
			anotherRegistry := obj.DeepCopy()
			anotherRegistry.ObjectMeta = metav1.ObjectMeta{Name: "another-registry"}
			Expect(k8sClient.Create(ctx, anotherRegistry)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, anotherRegistry)
				Eventually(func() error {
					r := &metalk8sv1alpha1.Registry{}
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(anotherRegistry), r)
				}, timeout, interval).ShouldNot(Succeed())
			})

			By("waiting for the cache to see the created registry")
			Eventually(func() error {
				r := &metalk8sv1alpha1.Registry{}
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(anotherRegistry), r)
			}, timeout, interval).Should(Succeed())

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if mTLS secret doesn't exist", func() {
			By("creating a registry with a missing mTLS secret")
			obj.Spec.Agent.Authentication.MTLS.CASecretRef.Name = "missing-secret"

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if server clusterIssuer doesn't exist", func() {
			By("creating a registry with a missing server clusterIssuer")
			obj.Spec.Server.CertificateIssuerRef.Name = "missing-clusterIssuer"

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if agent clusterIssuer doesn't exist", func() {
			By("creating a registry with a missing agent clusterIssuer")
			obj.Spec.Agent.CertificateIssuerRef.Kind = "ClusterIssuer"
			obj.Spec.Agent.CertificateIssuerRef.Name = "missing-clusterIssuer"

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if server issuer doesn't exist", func() {
			By("creating a registry with a missing server issuer")
			obj.Spec.Server.CertificateIssuerRef.Kind = "Issuer"

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should deny creation if agent issuer doesn't exist", func() {
			By("creating a registry with a missing agent issuer")
			obj.Spec.Agent.CertificateIssuerRef.Name = "missing-issuer"

			By("validating the creation")
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})

		It("Should allow creation when registry is valid and no other registry exists", func() {
			Eventually(func() error {
				_, err := validator.ValidateCreate(ctx, obj)
				return err
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When updating Registry under Validating Webhook", func() {
		It("Should allow update when LogLevel field is changed", func() {
			By("creating a registry with LogLevel field set to info")
			oldRegistry := obj.DeepCopy()
			oldRegistry.ObjectMeta = metav1.ObjectMeta{Name: "loglevel-registry"}
			oldRegistry.Spec.LogLevel = ptr.To("info")
			Expect(k8sClient.Create(ctx, oldRegistry)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, oldRegistry)
			})

			By("modifying the registry with different logLevel")
			newRegistry := oldRegistry.DeepCopy()
			newRegistry.Spec.LogLevel = ptr.To("debug")

			By("validating the update")
			_, err := validator.ValidateUpdate(ctx, oldRegistry, newRegistry)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny update when server ClusterIssuer does not exist", func() {
			By("creating a registry with a valid server ClusterIssuer")
			oldRegistry := obj.DeepCopy()
			oldRegistry.ObjectMeta = metav1.ObjectMeta{Name: "registry-non-existent-issuer"}
			Expect(k8sClient.Create(ctx, oldRegistry)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, oldRegistry)
			})

			By("updating the registry to reference a non-existent ClusterIssuer")
			newRegistry := oldRegistry.DeepCopy()
			newRegistry.Spec.Server.CertificateIssuerRef.Name = "non-existent-clusterissuer"

			By("validating the update")
			_, err := validator.ValidateUpdate(ctx, oldRegistry, newRegistry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("server clusterIssuer doesn't exist"))
		})
	})
})
