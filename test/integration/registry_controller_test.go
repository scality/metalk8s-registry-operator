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
	"github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:dupl
var _ = Describe("Registry Controller", func() {
	ctx := context.Background()
	timeout := 10 * time.Second
	interval := 1 * time.Second

	const secretNamespace = "metalk8s-secret"

	BeforeEach(func() {
		By("Loading the registry node agent manifests")
		registryNodeAgentManifests, err := os.ReadFile("../../dist/registry-node-agent.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(registryNodeAgent.LoadManifests(registryNodeAgentManifests)).To(Succeed())
		// We flush ValidatingWebhookConfigurations because in envtest there is no webhook server,
		// so the API server call times out and status is never updated.
		registryNodeAgent.ValidatingWebhookConfigurations = registryNodeAgent.ValidatingWebhookConfigurations[:0]

		By("Loading the registry server agent manifests")
		registryServerManifests, err := os.ReadFile("../../charts/registry-server.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(registryServer.LoadManifests(registryServerManifests)).To(Succeed())

		By("creating the namespace and CA secret required by the Registry")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: secretNamespace}}
		if err := k8sClient.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-agent-mtls-ca", Namespace: secretNamespace},
			Data:       map[string][]byte{"ca.crt": []byte("dummy-ca-cert")},
		}
		if err := k8sClient.Create(ctx, caSecret); err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		By("deleting the CA secret")
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-agent-mtls-ca", Namespace: secretNamespace},
		}
		Expect(k8sClient.Delete(ctx, caSecret)).To(Succeed())

		By("flushing the registry node agent resources")
		registryNodeAgent.Flush()
	})

	Context("When reconciling a new resource without selected nodes on registry", func() {
		It("should successfully reconcile the resource with a zero status", func() {
			resourceName := "test-new-resource"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("creating the custom resource for the Kind Registry")
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To("namespace-test-1"),
					NodeSelector: map[string]string{
						"registry": "test2",
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
							PullPolicy: ptr.To(corev1.PullNever),
							PullSecrets: []corev1.LocalObjectReference{
								{
									Name: "registry-pull-secret",
								},
							},
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{},
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:   "ghcr.io/scality",
							Name:       "metalk8s-registry-agent",
							Tag:        ptr.To("v1.2.3"),
							PullPolicy: ptr.To(corev1.PullAlways),
							PullSecrets: []corev1.LocalObjectReference{
								{
									Name: "registry-pull-secret",
								},
							},
						},
					},
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind Registry")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.SelectedNodes).To(BeEmpty())
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryUnavailable"),
					"Message":            Equal("The registry is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotAvailable"),
					"Message":            Equal("The registry agent is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(BeEmpty())

			By("deleting the custom resource for the Kind Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)
		})
	})

	Context("When reconciling a new resource with selected nodes on registry", func() {
		It("should successfully reconcile the resource with a non-zero status", func() {
			resourceName := "test-new-resource-with-nodes"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("Creating Nodes with labels matching the registry resource to add selected nodes")
			node3Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-3",
					Labels: map[string]string{
						"registry": "test3",
					},
				},
			}
			node4Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-4",
					Labels: map[string]string{
						"registry": "test3",
					},
				},
			}
			// Use your k8sClient to create the node
			Expect(k8sClient.Create(ctx, node3Resource)).To(Succeed())
			Expect(k8sClient.Create(ctx, node4Resource)).To(Succeed())

			node3Resource.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.3"}}
			node4Resource.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.4"}}
			Expect(k8sClient.Status().Update(ctx, node3Resource)).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, node4Resource)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node3Resource)
				_ = k8sClient.Delete(ctx, node4Resource)
			})

			By("creating the custom resource for the Kind Registry")
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To("namespace-test-2"),
					NodeSelector: map[string]string{
						"registry": "test3",
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
						CertificateIssuerRef: cmmetav1.ObjectReference{},
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
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
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind Registry")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeTrue()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(2)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.SelectedNodes).To(ConsistOf("node-3", "node-4"))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAvailable"),
					"Message":            Equal("The registry is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentAvailable"),
					"Message":            Equal("The registry agent is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-3"))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-4"))
			Expect(createdResource.Status.StatusPerNode["node-3"].Agent.Available).To(BeTrue())
			Expect(createdResource.Status.StatusPerNode["node-4"].Agent.Available).To(BeTrue())

			By("checking the generated Services")
			serviceResource1 := &corev1.Service{}
			serviceResource2 := &corev1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-server-node-3",
						Namespace: "namespace-test-2",
					},
					serviceResource1,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-server-node-4",
						Namespace: "namespace-test-2",
					},
					serviceResource2,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("checking the generated External Server Certificates")
			extServerCertificateResource1 := &cmv1.Certificate{}
			extServerCertificateResource2 := &cmv1.Certificate{}
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-external-server-node-3",
						Namespace: "namespace-test-2",
					},
					extServerCertificateResource1,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-external-server-node-4",
						Namespace: "namespace-test-2",
					},
					extServerCertificateResource2,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("checking the generated INternal Server Certificates")
			intServerCertificateResource1 := &cmv1.Certificate{}
			intServerCertificateResource2 := &cmv1.Certificate{}
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-server-node-3",
						Namespace: "namespace-test-2",
					},
					intServerCertificateResource1,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-server-node-4",
						Namespace: "namespace-test-2",
					},
					intServerCertificateResource2,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("checking the generated Internal Client Certificates")
			intClientCertificateResource1 := &cmv1.Certificate{}
			intClientCertificateResource2 := &cmv1.Certificate{}
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-client-node-3",
						Namespace: "namespace-test-2",
					},
					intClientCertificateResource1,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "rna-internal-client-node-4",
						Namespace: "namespace-test-2",
					},
					intClientCertificateResource2,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("checking the generated StatefulSets")
			statefulSetResource1 := &appsv1.StatefulSet{}
			statefulSetResource2 := &appsv1.StatefulSet{}
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "metalk8s-registry-node-agent-node-3",
						Namespace: "namespace-test-2",
					},
					statefulSetResource1,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      "metalk8s-registry-node-agent-node-4",
						Namespace: "namespace-test-2",
					},
					statefulSetResource2,
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("waiting for the registry reachability IPs in status")
			updatedResource := &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, updatedResource)).To(Succeed())
				g.Expect(updatedResource.Status.ClusterIP).NotTo(BeEmpty())
				g.Expect(updatedResource.Status.NodeIPs).To(ConsistOf("10.0.0.3", "10.0.0.4"))
			}, timeout, interval).Should(Succeed())

			By("checking the registry server ClusterIP Service")
			rsService := &corev1.Service{}
			Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "metalk8s-registry-server", Namespace: "namespace-test-2"},
				rsService,
			)).To(Succeed())
			Expect(rsService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(rsService.Spec.ClusterIP).To(Equal(updatedResource.Status.ClusterIP))

			By("checking the registry server cert SANs include the node IP, the ClusterIP and the Service DNS names")
			rsCert := &cmv1.Certificate{}
			Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "rs-external-server-node-3", Namespace: "namespace-test-2"},
				rsCert,
			)).To(Succeed())
			Expect(rsCert.Spec.IPAddresses).To(ContainElements("10.0.0.3", rsService.Spec.ClusterIP))
			Expect(rsCert.Spec.DNSNames).To(ContainElements(
				"metalk8s-registry-server",
				"metalk8s-registry-server.namespace-test-2",
				"metalk8s-registry-server.namespace-test-2.svc",
				"metalk8s-registry-server.namespace-test-2.svc.cluster.local",
			))

			By("checking the generated containerd mirror ConfigMap")
			mirrorCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "metalk8s-registry-containerd-mirror",
				Namespace: "namespace-test-2",
			}, mirrorCM)).To(Succeed())
			Expect(mirrorCM.Annotations).To(HaveKeyWithValue("registry.metalk8s.scality.com/certs-d-subdir", "_default"))
			Expect(mirrorCM.OwnerReferences).NotTo(BeEmpty())
			hosts := mirrorCM.Data["hosts.toml"]
			Expect(hosts).To(HavePrefix("[host.\"https://" + rsService.Spec.ClusterIP + ":5000\"]"))
			Expect(hosts).To(ContainSubstring("[host.\"https://10.0.0.3:5000\"]"))
			Expect(hosts).To(ContainSubstring("[host.\"https://10.0.0.4:5000\"]"))

			By("checking the registry server StatefulSet uses host networking and binds to the node IP")
			rsStatefulSet := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "metalk8s-registry-server-node-3",
				Namespace: "namespace-test-2",
			}, rsStatefulSet)).To(Succeed())
			Expect(rsStatefulSet.Spec.Template.Spec.HostNetwork).To(BeTrue())
			Expect(rsStatefulSet.Spec.Template.Spec.Containers[0].Env).To(
				ContainElement(corev1.EnvVar{Name: "HTTP_ADDR", Value: "10.0.0.3:5000"}),
			)

			By("checking the containerd mirror sync DaemonSet")
			syncDS := &appsv1.DaemonSet{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "metalk8s-registry-containerd-mirror-sync",
					Namespace: "namespace-test-2",
				}, syncDS)).To(Succeed())
				g.Expect(syncDS.OwnerReferences).NotTo(BeEmpty())
				container := syncDS.Spec.Template.Spec.Containers[0]
				g.Expect(container.Image).To(Equal(
					metalk8sv1alpha1.FileReflectorImageRegistry + "/" +
						metalk8sv1alpha1.FileReflectorImageName + ":" + metalk8sv1alpha1.FileReflectorImageTag,
				))
				g.Expect(container.Args).To(Equal([]string{
					"--source=/source",
					"--target=/target",
					"--file-mode=0644",
					"--owner=0:0",
				}))
				g.Expect(syncDS.Spec.Template.Spec.NodeSelector).To(
					Equal(map[string]string{"kubernetes.io/os": "linux"}),
				)
				g.Expect(container.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
				g.Expect(container.SecurityContext.Capabilities.Drop).To(
					Equal([]corev1.Capability{"ALL"}),
				)
				g.Expect(container.SecurityContext.Capabilities.Add).To(ConsistOf(
					corev1.Capability("DAC_OVERRIDE"),
					corev1.Capability("FOWNER"),
					corev1.Capability("CHOWN"),
				))
				g.Expect(container.VolumeMounts[0].MountPath).To(Equal("/source"))
				g.Expect(container.VolumeMounts[0].ReadOnly).To(BeTrue())
				volumes := syncDS.Spec.Template.Spec.Volumes
				g.Expect(volumes[0].ConfigMap.Name).To(Equal("metalk8s-registry-containerd-mirror"))
				g.Expect(volumes[0].ConfigMap.Items).To(BeEmpty())
				g.Expect(volumes[1].HostPath.Path).To(Equal("/etc/containerd/certs.d/_default"))
			}, timeout, interval).Should(Succeed())

			By("deleting the custom resource for the Kind Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)
		})
	})

	Context("When reconciling a new resource with no matching nodes on registry", func() {
		It("should successfully reconcile the resource with a zero status", func() {
			resourceName := "test-new-resource-with-no-matching-nodes"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("Creating Nodes with labels not matching the registry resource")
			node5Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-5",
					Labels: map[string]string{
						"registry": "wrong",
					},
				},
			}
			node6Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-6",
					Labels: map[string]string{
						"registry": "wrong",
					},
				},
			}
			// Use your k8sClient to create the node
			Expect(k8sClient.Create(ctx, node5Resource)).To(Succeed())
			Expect(k8sClient.Create(ctx, node6Resource)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node5Resource)
				_ = k8sClient.Delete(ctx, node6Resource)
			})

			By("creating the custom resource for the Kind Registry")
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To("namespace-test-3"),
					NodeSelector: map[string]string{
						"registry": "test4",
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
						CertificateIssuerRef: cmmetav1.ObjectReference{},
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
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
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind Registry")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.SelectedNodes).To(BeEmpty())
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryUnavailable"),
					"Message":            Equal("The registry is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotAvailable"),
					"Message":            Equal("The registry agent is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(BeEmpty())

			By("deleting the custom resource for the Kind Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)
		})
	})

	Context("When a node is removed from the registry (no longer matches selector)", func() {
		It("should reconcile and update status to reflect the remaining selected nodes", func() {
			resourceName := "test-resource-with-removing-a-node"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("Creating Nodes with labels matching the registry resource to add selected nodes")
			node7Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-7",
					Labels: map[string]string{
						"registry": "test5",
					},
				},
			}
			node8Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-8",
					Labels: map[string]string{
						"registry": "test5",
					},
				},
			}
			// Use your k8sClient to create the node
			Expect(k8sClient.Create(ctx, node7Resource)).To(Succeed())
			Expect(k8sClient.Create(ctx, node8Resource)).To(Succeed())

			node7Resource.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.7"}}
			node8Resource.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.8"}}
			Expect(k8sClient.Status().Update(ctx, node7Resource)).To(Succeed())
			Expect(k8sClient.Status().Update(ctx, node8Resource)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node7Resource)
				_ = k8sClient.Delete(ctx, node8Resource)
			})

			By("creating the custom resource for the Kind Registry")
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To("namespace-test-4"),
					NodeSelector: map[string]string{
						"registry": "test5",
					},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:   "ghcr.io/scality",
							Name:       "metalk8s-registry-server",
							Tag:        ptr.To("v2.0.0"),
							PullPolicy: ptr.To(corev1.PullIfNotPresent),
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{},
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:   "ghcr.io/scality",
							Name:       "metalk8s-registry-agent",
							Tag:        ptr.To("v2.3.4"),
							PullPolicy: ptr.To(corev1.PullIfNotPresent),
						},
					},
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind Registry")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeTrue()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(2)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.SelectedNodes).To(ConsistOf("node-7", "node-8"))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAvailable"),
					"Message":            Equal("The registry is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentAvailable"),
					"Message":            Equal("The registry agent is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-7"))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-8"))
			Expect(createdResource.Status.StatusPerNode["node-7"].Agent.Available).To(BeTrue())
			Expect(createdResource.Status.StatusPerNode["node-8"].Agent.Available).To(BeTrue())

			By("Removing a Node from the registry")
			node8Resource = &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-8",
					Labels: map[string]string{
						"registry": "test5-removed",
					},
				},
			}
			// Use your k8sClient to update the node
			Expect(k8sClient.Update(ctx, node8Resource)).To(Succeed())

			By("checking the custom resource for the Kind Registry")
			createdResource = &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, createdResource)).To(Succeed())
				g.Expect(createdResource.Status.SelectedNodes).To(ConsistOf("node-7"))
			}, timeout, interval).Should(Succeed())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeTrue()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(1)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAvailable"),
					"Message":            Equal("The registry is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentAvailable"),
					"Message":            Equal("The registry agent is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-7"))
			Expect(createdResource.Status.StatusPerNode).To(Not(HaveKey("node-8")))
			Expect(createdResource.Status.StatusPerNode["node-7"].Agent.Available).To(BeTrue())

			By("checking the containerd mirror ConfigMap dropped the removed node's host")
			mirrorCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "metalk8s-registry-containerd-mirror",
				Namespace: "namespace-test-4",
			}, mirrorCM)).To(Succeed())
			hosts := mirrorCM.Data["hosts.toml"]
			Expect(hosts).To(ContainSubstring("[host.\"https://10.0.0.7:5000\"]"))
			// Match the full host entry: the ClusterIP randomly assigned by the API
			// server (e.g. 10.0.0.85) could contain "10.0.0.8" as a substring.
			Expect(hosts).NotTo(ContainSubstring("[host.\"https://10.0.0.8:5000\"]"))

			By("deleting the custom resource for the Kind Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)
		})
	})

	Context("When all nodes are removed from the registry (no longer matches selector)", func() {
		It("should reconcile and update status to reflect the no selected nodes", func() {
			resourceName := "test-resource-with-removing-all-nodes"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("Creating Nodes with labels matching the registry resource to add selected nodes")
			node9Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-9",
					Labels: map[string]string{
						"registry": "test6",
					},
				},
			}
			node10Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-10",
					Labels: map[string]string{
						"registry": "test6",
					},
				},
			}
			// Use your k8sClient to create the node
			Expect(k8sClient.Create(ctx, node9Resource)).To(Succeed())
			Expect(k8sClient.Create(ctx, node10Resource)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node9Resource)
				_ = k8sClient.Delete(ctx, node10Resource)
			})

			By("creating the custom resource for the Kind Registry")
			resource := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To("namespace-test-5"),
					NodeSelector: map[string]string{
						"registry": "test6",
					},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:   "ghcr.io/scality",
							Name:       "metalk8s-registry-server",
							Tag:        ptr.To("v3.0.0"),
							PullPolicy: ptr.To(corev1.PullIfNotPresent),
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{},
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:   "ghcr.io/scality",
							Name:       "metalk8s-registry-agent",
							Tag:        ptr.To("v3.4.5"),
							PullPolicy: ptr.To(corev1.PullIfNotPresent),
						},
					},
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind Registry")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeTrue()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(Equal(2)))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.SelectedNodes).To(ConsistOf("node-9", "node-10"))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAvailable"),
					"Message":            Equal("The registry is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionTrue),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentAvailable"),
					"Message":            Equal("The registry agent is available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-9"))
			Expect(createdResource.Status.StatusPerNode).To(HaveKey("node-10"))
			Expect(createdResource.Status.StatusPerNode["node-9"].Agent.Available).To(BeTrue())
			Expect(createdResource.Status.StatusPerNode["node-10"].Agent.Available).To(BeTrue())

			By("Removing all Nodes from the registry")
			node9Resource = &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-9",
					Labels: map[string]string{
						"registry": "test6-removed",
					},
				},
			}
			node10Resource = &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-10",
					Labels: map[string]string{
						"registry": "test6-removed",
					},
				},
			}
			// Use your k8sClient to update the nodes
			Expect(k8sClient.Update(ctx, node9Resource)).To(Succeed())
			Expect(k8sClient.Update(ctx, node10Resource)).To(Succeed())

			By("checking the custom resource for the Kind Registry")
			createdResource = &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, createdResource)).To(Succeed())
				g.Expect(createdResource.Status.SelectedNodes).To(BeEmpty())
			}, timeout, interval).Should(Succeed())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Available).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Ready).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicas).To(HaveValue(BeZero()))
			Expect(createdResource.Status.ReadyServerReplicas).To(HaveValue(BeZero()))
			Expect(createdResource.Status.ReadyAgentReplicas).To(HaveValue(BeZero()))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Available"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryUnavailable"),
					"Message":            Equal("The registry is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("Ready"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryNotReady"),
					"Message":            Equal("The registry is not ready."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentAvailable"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotAvailable"),
					"Message":            Equal("The registry agent is not available."),
				}),
			))
			Expect(createdResource.Status.Conditions).To(ContainElement(
				gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":               Equal("AgentReady"),
					"Status":             Equal(metav1.ConditionFalse),
					"ObservedGeneration": Equal(int64(1)),
					"Reason":             Equal("RegistryAgentNotReady"),
					"Message":            Equal("The registry agent is not ready."),
				}),
			))
			Expect(createdResource.Status.StatusPerNode).To(BeEmpty())

			By("deleting the custom resource for the Kind Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)
		})
	})

	Context("When reconciling the containerd mirror ConfigMap with a server CA", func() {
		It("includes ca.crt and deletes the ConfigMap when mirrorPropagation is disabled", func() {
			resourceName := "test-mirror-ca"
			namespace := "namespace-mirror-ca"
			cmName := "metalk8s-registry-containerd-mirror"

			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   "ca-node-a",
				Labels: map[string]string{"registry": "mirror-ca"},
			}}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			node.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.2.1"}}
			Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, node) })

			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
			caSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "rs-external-server-ca-node-a", Namespace: namespace},
				Data:       map[string][]byte{"ca.crt": []byte("dummy-server-ca")},
			}
			Expect(k8sClient.Create(ctx, caSecret)).To(Succeed())

			resource := &metalk8sv1alpha1.Registry{ObjectMeta: metav1.ObjectMeta{Name: resourceName}}
			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To(namespace),
					NodeSelector:  map[string]string{"registry": "mirror-ca"},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-server",
							Tag:      ptr.To("v1.0.0"),
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-agent",
							Tag:      ptr.To("v1.2.3"),
						},
					},
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, resource) })

			By("waiting for the node IPs in status")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, createdResource)).To(Succeed())
				g.Expect(createdResource.Status.NodeIPs).To(ConsistOf("10.0.2.1"))
			}, timeout, interval).Should(Succeed())

			By("checking ca.crt is present in the ConfigMap")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("ca.crt", "dummy-server-ca"))

			By("disabling mirrorPropagation deletes the ConfigMap")
			updatedResource := &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updatedResource)).To(Succeed())
				updatedResource.Spec.MirrorPropagation = &metalk8sv1alpha1.MirrorPropagationSpec{Enabled: false}
				g.Expect(k8sClient.Update(ctx, updatedResource)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			// Sync on the conditions observing the new generation: the ConfigMap
			// deletion happens before the status is patched.
			Eventually(func(g Gomega) {
				current := &metalk8sv1alpha1.Registry{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, current)).To(Succeed())
				g.Expect(current.Status.Conditions).To(ContainElement(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Type":               Equal("Available"),
						"ObservedGeneration": Equal(updatedResource.Generation),
					}),
				))
			}, timeout, interval).Should(Succeed())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("disabling mirrorPropagation deletes the sync DaemonSet too")
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{Name: "metalk8s-registry-containerd-mirror-sync", Namespace: namespace},
					&appsv1.DaemonSet{},
				)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When reconciling the containerd mirror ConfigMap with no selected nodes", func() {
		It("keeps only the ClusterIP host", func() {
			resourceName := "test-mirror-no-nodes"
			namespace := "namespace-mirror-nonodes"
			cmName := "metalk8s-registry-containerd-mirror"

			resource := &metalk8sv1alpha1.Registry{ObjectMeta: metav1.ObjectMeta{Name: resourceName}}
			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To(namespace),
					NodeSelector:  map[string]string{"registry": "mirror-none"},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-server",
							Tag:      ptr.To("v1.0.0"),
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-agent",
							Tag:      ptr.To("v1.2.3"),
						},
					},
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, resource) })

			By("waiting for the ClusterIP in status")
			createdResource := &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, createdResource)).To(Succeed())
				g.Expect(createdResource.Status.ClusterIP).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
			Expect(createdResource.Status.NodeIPs).To(BeEmpty())

			By("checking the registry server Service matches the status ClusterIP")
			rsService := &corev1.Service{}
			Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "metalk8s-registry-server", Namespace: namespace},
				rsService,
			)).To(Succeed())
			Expect(rsService.Spec.ClusterIP).To(Equal(createdResource.Status.ClusterIP))

			By("checking the ConfigMap only contains the ClusterIP host")
			expectedHosts := "[host.\"https://" + createdResource.Status.ClusterIP + ":5000\"]\n" +
				"  capabilities = [\"pull\", \"resolve\"]\n" +
				"  ca = \"ca.crt\"\n"
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm)).To(Succeed())
			Expect(cm.Data["hosts.toml"]).To(Equal(expectedHosts))
		})
	})

	Context("When reconciling the containerd mirror sync DaemonSet with custom fields", func() {
		It("propagates image, path, scheduling and ignore paths to the DaemonSet", func() {
			resourceName := "test-mirror-sync-custom"
			namespace := "namespace-mirror-sync-custom"

			resource := &metalk8sv1alpha1.Registry{ObjectMeta: metav1.ObjectMeta{Name: resourceName}}
			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = metalk8sv1alpha1.RegistrySpec{
					LogLevel:      ptr.To("info"),
					ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
					SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
					Namespace:     ptr.To(namespace),
					NodeSelector:  map[string]string{"registry": "mirror-sync-custom"},
					MirrorPropagation: &metalk8sv1alpha1.MirrorPropagationSpec{
						Enabled: true,
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry:    "registry.example.com",
							Name:        "custom-reflector",
							Tag:         ptr.To("v9.9.9"),
							PullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
						},
						ContainerdConfigPath: "/var/lib/containerd/certs.d",
						NodeSelector:         map[string]string{"kubernetes.io/arch": "amd64"},
						Tolerations: []corev1.Toleration{{
							Key:      "node-role.kubernetes.io/bootstrap",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						}},
						IgnorePaths: []string{"legacy.invalid", "other.invalid"},
					},
					Server: metalk8sv1alpha1.RegistryServerSpec{
						CertificateIssuerRef: cmmetav1.ObjectReference{
							Name: "registry-server-issuer",
							Kind: "ClusterIssuer",
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-server",
							Tag:      ptr.To("v1.0.0"),
						},
					},
					Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
						Authentication: metalk8sv1alpha1.AuthenticationSpec{
							MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
								CASecretRef: corev1.SecretReference{
									Name:      "registry-agent-mtls-ca",
									Namespace: secretNamespace,
								},
							},
						},
						Image: &metalk8sv1alpha1.ImageSpec{
							Registry: "ghcr.io/scality",
							Name:     "metalk8s-registry-agent",
							Tag:      ptr.To("v1.2.3"),
						},
					},
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, resource) })

			By("waiting for the mirror sync available status")
			Eventually(func(g Gomega) {
				updated := &metalk8sv1alpha1.Registry{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updated)).To(Succeed())
				g.Expect(updated.Status.MirrorSyncAvailable).To(HaveValue(BeTrue()))
			}, timeout, interval).Should(Succeed())

			By("checking the sync DaemonSet carries the custom fields")
			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "metalk8s-registry-containerd-mirror-sync",
				Namespace: namespace,
			}, ds)).To(Succeed())
			container := ds.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("registry.example.com/custom-reflector:v9.9.9"))
			Expect(container.Args).To(Equal([]string{
				"--source=/source",
				"--target=/target",
				"--ignore=legacy.invalid",
				"--ignore=other.invalid",
				"--file-mode=0644",
				"--owner=0:0",
			}))
			Expect(ds.Spec.Template.Spec.NodeSelector).To(
				Equal(map[string]string{"kubernetes.io/arch": "amd64"}),
			)
			Expect(ds.Spec.Template.Spec.Tolerations).To(HaveLen(1))
			Expect(ds.Spec.Template.Spec.Tolerations[0].Key).To(
				Equal("node-role.kubernetes.io/bootstrap"),
			)
			Expect(ds.Spec.Template.Spec.ImagePullSecrets).To(
				ConsistOf(corev1.LocalObjectReference{Name: "regcred"}),
			)
			Expect(ds.Spec.Template.Spec.Volumes[1].HostPath.Path).To(
				Equal("/var/lib/containerd/certs.d/_default"),
			)

			By("clearing mirrorPropagation drops the tolerations and pull secrets")
			updatedResource := &metalk8sv1alpha1.Registry{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, updatedResource)).To(Succeed())
				updatedResource.Spec.MirrorPropagation = nil
				g.Expect(k8sClient.Update(ctx, updatedResource)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			// Sync on the conditions observing the new generation: the DaemonSet is
			// reconciled before the status is patched.
			Eventually(func(g Gomega) {
				current := &metalk8sv1alpha1.Registry{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName}, current)).To(Succeed())
				g.Expect(current.Status.Conditions).To(ContainElement(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Type":               Equal("Available"),
						"ObservedGeneration": Equal(updatedResource.Generation),
					}),
				))
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "metalk8s-registry-containerd-mirror-sync",
				Namespace: namespace,
			}, ds)).To(Succeed())
			Expect(ds.Spec.Template.Spec.Tolerations).To(BeEmpty())
			Expect(ds.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
		})
	})
})
