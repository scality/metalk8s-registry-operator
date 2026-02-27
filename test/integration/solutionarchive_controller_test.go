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
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// nolint:dupl
var _ = Describe("SolutionArchive Controller", func() {
	ctx := context.Background()
	timeout := 10 * time.Second
	interval := 1 * time.Second

	const secretNamespace = "metalk8s-secret"
	const registryNamespace = "metalk8s-registry"
	const registryName = "test-registry"
	registryNamespacedName := types.NamespacedName{
		Name: registryName,
	}

	var registryResource *metalk8sv1alpha1.Registry

	registry := &metalk8sv1alpha1.Registry{}

	BeforeEach(func() {
		By("Loading the registry node agent manifests")
		manifestPath := filepath.Join("..", "..", "dist", "registry-node-agent.yaml")
		Expect(registryNodeAgent.LoadManifestsFromFile(manifestPath)).To(Succeed())
		// We flush ValidatingWebhookConfigurations because in envtest there is no webhook server,
		// so the API server call times out and status is never updated.
		registryNodeAgent.ValidatingWebhookConfigurations = registryNodeAgent.ValidatingWebhookConfigurations[:0]

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

		By("creating a Registry resource")
		registryResource = &metalk8sv1alpha1.Registry{
			ObjectMeta: metav1.ObjectMeta{
				Name: registryName,
			},
			Spec: metalk8sv1alpha1.RegistrySpec{
				LogLevel:      ptr.To("info"),
				ArchivesPath:  ptr.To("/srv/scality/metalk8s/archives"),
				SolutionsPath: ptr.To("/srv/scality/metalk8s/solutions"),
				Namespace:     ptr.To(registryNamespace),
				NodeSelector: map[string]string{
					"registry": "test1",
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
				},
			},
		}
		Expect(k8sClient.Create(ctx, registryResource)).To(Succeed())

		// Waiting for the registry resource to be created
		Eventually(func() bool {
			err := k8sClient.Get(ctx, registryNamespacedName, registry)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		time.Sleep(1 * time.Second)
	})

	AfterEach(func() {
		By("deleting the Registry resource")
		Expect(k8sClient.Delete(ctx, registryResource)).To(Succeed())

		// Waiting for the registry resource to be deleted
		Eventually(func() bool {
			err := k8sClient.Get(ctx, registryNamespacedName, registry)
			return err != nil
		}, timeout, interval).Should(BeTrue())

		By("deleting the CA secret")
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-agent-mtls-ca", Namespace: secretNamespace},
		}
		Expect(k8sClient.Delete(ctx, caSecret)).To(Succeed())

		// Wait for the registry resource to be deleted
		time.Sleep(1 * time.Second)

		By("flushing the registry node agent resources")
		registryNodeAgent.Flush()
	})

	Context("When reconciling a new resource without selected nodes on registry", func() {
		It("should successfully reconcile the resource with a zero status", func() {
			resourceName := "test-new-resource"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("creating the custom resource for the Kind SolutionArchive")
			resource := &metalk8sv1alpha1.SolutionArchive{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = rnav1alpha1.SolutionArchiveSpec{
					Name:    "solution-1",
					Version: "1.2.0",
					Validation: rnav1alpha1.SolutionArchiveValidation{
						Checksum: rnav1alpha1.SolutionArchiveChecksum{
							Type:  "sha256",
							Value: "ce775a33b30ae640d521df1fad60868fa701707ffdc4d8b4ca7ab60edfd05c26",
						},
					},
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind SolutionArchive")
			createdResource := &metalk8sv1alpha1.SolutionArchive{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Served).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicated).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.ServedReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.TargetReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.StatusPerNodeSolutionArchive).To(BeEmpty())
			Expect(createdResource.Status.NodeSolutionArchives).To(BeEmpty())
			Expect(createdResource.Status.Conditions).To(BeEmpty())
		})
	})

	Context("When reconciling a new resource with selected nodes on registry", func() {
		It("should successfully reconcile the resource with a non-zero status", func() {
			resourceName := "test-new-resource-with-nodes"
			typeNamespacedName := types.NamespacedName{
				Name: resourceName,
			}

			By("Creating Nodes with labels matching the registry resource to add selected nodes")
			node1Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"registry": "test1",
					},
				},
			}
			node2Resource := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"registry": "test1",
					},
				},
			}
			// Use your k8sClient to create the node
			Expect(k8sClient.Create(ctx, node1Resource)).To(Succeed())
			Expect(k8sClient.Create(ctx, node2Resource)).To(Succeed())

			// Wait for the nodes to be created
			time.Sleep(1 * time.Second)

			By("Verifying the registry resource to have selected nodes")
			registryResourceSearch := &metalk8sv1alpha1.Registry{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: registryResource.Name}, registryResourceSearch)
				return err == nil &&
					registryResourceSearch.Status.SelectedNodes != nil &&
					len(registryResourceSearch.Status.SelectedNodes) == 2
			}, timeout, interval).Should(BeTrue())

			By("creating the custom resource for the Kind SolutionArchive")
			resource := &metalk8sv1alpha1.SolutionArchive{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
			}

			_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, resource, func() error {
				resource.Spec = rnav1alpha1.SolutionArchiveSpec{
					Name:    "solution-2",
					Version: "1.2.0",
					Validation: rnav1alpha1.SolutionArchiveValidation{
						Checksum: rnav1alpha1.SolutionArchiveChecksum{
							Type:  "sha256",
							Value: "ce775a33b30ae640d521df1fad60868fa701707ffdc4d8b4ca7ab60edfd05c26",
						},
					},
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Wait for all reconciliations loop to be done
			time.Sleep(1 * time.Second)

			By("checking the custom resource for the Kind SolutionArchive")
			createdResource := &metalk8sv1alpha1.SolutionArchive{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdResource)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdResource.Spec).To(Equal(resource.Spec))
			Expect(createdResource.Status.Served).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.Replicated).To(HaveValue(BeFalse()))
			Expect(createdResource.Status.ServedReplicas).To(HaveValue(Equal(0)))
			Expect(createdResource.Status.TargetReplicas).To(HaveValue(Equal(2)))
			//			Expect(createdResource.Status.StatusPerNodeSolutionArchive).To(BeEmpty())
			Expect(createdResource.Status.NodeSolutionArchives).To(ContainElements(
				"solution-2-1.2.0-node-1",
				"solution-2-1.2.0-node-2",
			))
			Expect(createdResource.Status.Conditions).To(BeEmpty())

			By("deleting the nodes")
			Expect(k8sClient.Delete(ctx, node1Resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, node2Resource)).To(Succeed())
		})
	})
})
