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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/internal/utils"
)

var _ = Describe("Registry Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		registry := &metalk8sv1alpha1.Registry{}

		BeforeEach(func() {
			By("creating the namespace and CA secret required by the Registry")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-namespace"}}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: ns.Name}, &corev1.Namespace{}); err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}
			caSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "registry-agent-mtls-ca", Namespace: "my-namespace"},
				Data:       map[string][]byte{"ca.crt": []byte("dummy-ca-cert")},
			}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: caSecret.Name, Namespace: caSecret.Namespace}, &corev1.Secret{}); err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, caSecret)).To(Succeed())
			}

			By("creating the custom resource for the Kind Registry")
			err := k8sClient.Get(ctx, typeNamespacedName, registry)
			if err != nil && errors.IsNotFound(err) {
				resource := &metalk8sv1alpha1.Registry{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: metalk8sv1alpha1.RegistrySpec{
						Namespace: ptr.To("my-namespace"),
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
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
			// Wait for the resource to be created
			time.Sleep(1 * time.Second)
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &metalk8sv1alpha1.Registry{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Registry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			registryNodeAgent := utils.NewRegistryComponent(ctx)
			registryNodeAgentManifests, err := os.ReadFile("../../dist/registry-node-agent.yaml")
			Expect(err).NotTo(HaveOccurred())
			Expect(registryNodeAgent.LoadManifests(registryNodeAgentManifests)).To(Succeed())

			controllerReconciler := &RegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RNA:    registryNodeAgent,
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
