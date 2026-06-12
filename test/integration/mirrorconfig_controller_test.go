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

package k8s

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

var _ = Describe("MirrorConfig Controller", func() {
	ctx := context.Background()
	timeout := 10 * time.Second
	interval := 1 * time.Second

	BeforeEach(func() {
		By("Loading the registry node agent and registry server manifests")
		registryNodeAgentManifests, err := os.ReadFile("../../dist/registry-node-agent.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(registryNodeAgent.LoadManifests(registryNodeAgentManifests)).To(Succeed())
		// We flush ValidatingWebhookConfigurations because in envtest there is no webhook server,
		// so the API server call times out and status is never updated.
		registryNodeAgent.ValidatingWebhookConfigurations = registryNodeAgent.ValidatingWebhookConfigurations[:0]

		registryServerManifests, err := os.ReadFile("../../charts/registry-server.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(registryServer.LoadManifests(registryServerManifests)).To(Succeed())
	})

	AfterEach(func() {
		By("flushing the registry components")
		registryNodeAgent.Flush()
		registryServer.Flush()
	})

	Context("When reconciling a MirrorConfig with a Registry deployed", func() {
		It("renders the mirror ConfigMap in the workload namespace", func() {
			registryName := "test-mirrorconfig-registry"
			registryNamespace := "namespace-mirrorconfig"
			workloadNamespace := "team-mirrorconfig"

			By("creating a node with an InternalIP")
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   "mc-node-a",
				Labels: map[string]string{"registry": "mirrorconfig"},
			}}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			node.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.3.1"}}
			Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, node) })

			By("seeding the registry server CA secret and the agent mTLS CA secret")
			Expect(k8sClient.Create(
				ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: registryNamespace}},
			)).To(Succeed())
			caSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "rs-external-server-mc-node-a", Namespace: registryNamespace},
				Data:       map[string][]byte{"ca.crt": []byte("dummy-mirror-ca")},
			}
			Expect(k8sClient.Create(ctx, caSecret)).To(Succeed())
			mtlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "registry-agent-mtls-ca", Namespace: registryNamespace},
				Data:       map[string][]byte{"ca.crt": []byte("dummy-mtls-ca")},
			}
			Expect(k8sClient.Create(ctx, mtlsSecret)).To(Succeed())

			By("creating the Registry")
			registry := &metalk8sv1alpha1.Registry{ObjectMeta: metav1.ObjectMeta{Name: registryName}}
			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, registry, func() error {
				registry.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To(registryNamespace),
					NodeSelector:  map[string]string{"registry": "mirrorconfig"},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: registryNamespace,
								},
							},
						},
					},
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, registry) })

			By("creating the MirrorConfig in the workload namespace")
			Expect(k8sClient.Create(
				ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: workloadNamespace}},
			)).To(Succeed())
			mirrorConfig := &metalk8sv1alpha1.MirrorConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "team-mirror", Namespace: workloadNamespace},
				Spec: metalk8sv1alpha1.MirrorConfigSpec{
					Registries: []metalk8sv1alpha1.MirrorRegistry{{Prefix: "docker.io"}, {Prefix: "ghcr.io"}},
				},
			}
			Expect(k8sClient.Create(ctx, mirrorConfig)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, mirrorConfig) })

			expectedEndpoint := "metalk8s-registry-server." + registryNamespace + ".svc:5000"

			By("waiting for the MirrorConfig to report the registry not ready, without rendering")
			Eventually(func(g Gomega) {
				updated := &metalk8sv1alpha1.MirrorConfig{}
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, updated,
				)).To(Succeed())
				g.Expect(updated.Status.Conditions).NotTo(BeEmpty())
				g.Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
				g.Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
				g.Expect(updated.Status.Conditions[0].Reason).To(Equal("RegistryNotReady"))
			}, timeout, interval).Should(Succeed())
			err = k8sClient.Get(
				ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, &corev1.ConfigMap{},
			)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("marking the registry StatefulSets ready")
			for _, stsName := range []string{
				"metalk8s-registry-node-agent-mc-node-a",
				"metalk8s-registry-server-mc-node-a",
			} {
				Eventually(func(g Gomega) {
					sts := &appsv1.StatefulSet{}
					g.Expect(k8sClient.Get(
						ctx, types.NamespacedName{Name: stsName, Namespace: registryNamespace}, sts,
					)).To(Succeed())
					sts.Status.Replicas = 1
					sts.Status.ReadyReplicas = 1
					sts.Status.AvailableReplicas = 1
					sts.Status.ObservedGeneration = sts.Generation
					g.Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())
				}, timeout, interval).Should(Succeed())
			}

			By("waiting for the Registry to become ready")
			Eventually(func(g Gomega) {
				currentRegistry := &metalk8sv1alpha1.Registry{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: registryName}, currentRegistry)).To(Succeed())
				g.Expect(currentRegistry.Status.Ready).To(HaveValue(BeTrue()))
			}, timeout, interval).Should(Succeed())

			By("waiting for the MirrorConfig status to converge once the registry is ready")
			Eventually(func(g Gomega) {
				updated := &metalk8sv1alpha1.MirrorConfig{}
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, updated,
				)).To(Succeed())
				g.Expect(updated.Status.ObservedRegistries).To(Equal([]string{"docker.io", "ghcr.io"}))
				g.Expect(updated.Status.CASecretRef).NotTo(BeNil())
				g.Expect(updated.Status.CASecretRef.Name).To(Equal("rs-external-server-mc-node-a"))
				g.Expect(updated.Status.CASecretRef.Namespace).To(Equal(registryNamespace))
				g.Expect(updated.Status.Conditions).NotTo(BeEmpty())
				g.Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
				g.Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
				g.Expect(updated.Status.Conditions[0].Reason).To(Equal("ConfigMapRendered"))
			}, timeout, interval).Should(Succeed())

			By("checking the rendered ConfigMap")
			// The status reflects the rendered state: the ConfigMap content can be
			// asserted directly.
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(
				ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, cm,
			)).To(Succeed())
			Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "registry-operator"))
			Expect(cm.OwnerReferences).To(HaveLen(1))
			Expect(cm.OwnerReferences[0].Kind).To(Equal("MirrorConfig"))
			Expect(cm.Data).To(HaveKeyWithValue("endpoint", expectedEndpoint))
			Expect(cm.Data).To(HaveKeyWithValue("ca.crt", "dummy-mirror-ca"))
			Expect(cm.Data["registries.conf"]).To(ContainSubstring(
				"prefix = \"docker.io\"\nlocation = \"" + expectedEndpoint + "/docker.io\"",
			))
			Expect(cm.Data["registries.conf"]).To(ContainSubstring("prefix = \"ghcr.io\""))

			By("re-rendering when spec.registries changes")
			Eventually(func(g Gomega) {
				current := &metalk8sv1alpha1.MirrorConfig{}
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, current,
				)).To(Succeed())
				current.Spec.Registries = append(current.Spec.Registries, metalk8sv1alpha1.MirrorRegistry{Prefix: "quay.io"})
				g.Expect(k8sClient.Update(ctx, current)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				updated := &metalk8sv1alpha1.MirrorConfig{}
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, updated,
				)).To(Succeed())
				g.Expect(updated.Status.ObservedRegistries).To(ContainElement("quay.io"))
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Get(
				ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, cm,
			)).To(Succeed())
			Expect(cm.Data["registries.conf"]).To(ContainSubstring("prefix = \"quay.io\""))

			By("propagating a CA rotation to the ConfigMap")
			Eventually(func(g Gomega) {
				currentSecret := &corev1.Secret{}
				g.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{Name: "rs-external-server-mc-node-a", Namespace: registryNamespace},
					currentSecret,
				)).To(Succeed())
				currentSecret.Data["ca.crt"] = []byte("rotated-mirror-ca")
				g.Expect(k8sClient.Update(ctx, currentSecret)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			// Exception to the status-first rule: the status carries no trace of
			// the CA content, so we synchronize on the ConfigMap directly.
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, cm,
				)).To(Succeed())
				g.Expect(cm.Data).To(HaveKeyWithValue("ca.crt", "rotated-mirror-ca"))
			}, timeout, interval).Should(Succeed())

			By("keeping the ConfigMap when the registry becomes not ready")
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{Name: "metalk8s-registry-server-mc-node-a", Namespace: registryNamespace},
					sts,
				)).To(Succeed())
				sts.Status.AvailableReplicas = 0
				sts.Status.ReadyReplicas = 0
				g.Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				updated := &metalk8sv1alpha1.MirrorConfig{}
				g.Expect(k8sClient.Get(
					ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, updated,
				)).To(Succeed())
				g.Expect(updated.Status.Conditions).NotTo(BeEmpty())
				g.Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
				g.Expect(updated.Status.Conditions[0].Reason).To(Equal("RegistryNotReady"))
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Get(
				ctx, types.NamespacedName{Name: "team-mirror", Namespace: workloadNamespace}, cm,
			)).To(Succeed())
			Expect(cm.Data["registries.conf"]).To(ContainSubstring("prefix = \"quay.io\""))
		})
	})
})
