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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

var _ = Describe("MirrorConfig Controller", func() {
	Context("When reconciling with multiple Registries", func() {
		It("marks the MirrorConfig not ready and renders no ConfigMap", func() {
			ctx := context.Background()

			By("creating two Registries (no validation webhook in this suite)")
			for _, name := range []string{"multi-registry-a", "multi-registry-b"} {
				registry := &metalk8sv1alpha1.Registry{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Spec: metalk8sv1alpha1.RegistrySpec{
						NodeSelector: map[string]string{"registry": "multi"},
					},
				}
				Expect(k8sClient.Create(ctx, registry)).To(Succeed())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, registry) })
			}

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mirrorconfig-multi"}}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: ns.Name}, &corev1.Namespace{}); err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}

			mirrorConfig := &metalk8sv1alpha1.MirrorConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "team-mirror", Namespace: ns.Name},
				Spec: metalk8sv1alpha1.MirrorConfigSpec{
					Registries: []metalk8sv1alpha1.MirrorRegistry{{Prefix: "docker.io"}},
				},
			}
			Expect(k8sClient.Create(ctx, mirrorConfig)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, mirrorConfig) })

			reconciler := &MirrorConfigReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			updated := &metalk8sv1alpha1.MirrorConfig{}
			// k8sClient is the manager's cached client: reconcile until the cache
			// has caught up with the freshly created objects.
			Eventually(func(g Gomega) {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace},
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Conditions).To(HaveLen(1))
				g.Expect(updated.Status.Conditions[0].Reason).To(Equal("MultipleRegistries"))
			}, 10*time.Second, time.Second).Should(Succeed())

			Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

			err := k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("When the registry is ready but the CA secret is missing", func() {
		It("returns an error and marks the MirrorConfig not ready", func() {
			ctx := context.Background()

			registryList := &metalk8sv1alpha1.RegistryList{}
			Expect(k8sClient.List(ctx, registryList)).To(Succeed())
			if len(registryList.Items) > 0 {
				Skip("a Registry exists in the shared envtest, skipping")
			}

			By("creating a ready Registry without any server cert secret")
			registry := &metalk8sv1alpha1.Registry{
				ObjectMeta: metav1.ObjectMeta{Name: "ready-registry"},
				Spec: metalk8sv1alpha1.RegistrySpec{
					NodeSelector: map[string]string{"registry": "ready"},
				},
			}
			Expect(k8sClient.Create(ctx, registry)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, registry) })
			Eventually(func(g Gomega) {
				current := &metalk8sv1alpha1.Registry{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: registry.Name}, current)).To(Succeed())
				current.Status.Ready = ptr.To(true)
				current.Status.SelectedNodes = []string{"node-1"}
				g.Expect(k8sClient.Status().Update(ctx, current)).To(Succeed())
			}, 10*time.Second, time.Second).Should(Succeed())

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mirrorconfig-ca-missing"}}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: ns.Name}, &corev1.Namespace{}); err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}

			mirrorConfig := &metalk8sv1alpha1.MirrorConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "team-mirror", Namespace: ns.Name},
				Spec: metalk8sv1alpha1.MirrorConfigSpec{
					Registries: []metalk8sv1alpha1.MirrorRegistry{{Prefix: "docker.io"}},
				},
			}
			Expect(k8sClient.Create(ctx, mirrorConfig)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, mirrorConfig) })

			reconciler := &MirrorConfigReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			updated := &metalk8sv1alpha1.MirrorConfig{}
			// k8sClient is the manager's cached client: reconcile until the cache
			// has caught up with the freshly created objects.
			Eventually(func(g Gomega) {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace},
				})
				g.Expect(err).To(MatchError(ContainSubstring("no registry server CA secret")))
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Conditions).To(HaveLen(1))
				g.Expect(updated.Status.Conditions[0].Reason).To(Equal("RegistryNotReady"))
			}, 10*time.Second, time.Second).Should(Succeed())

			Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

			err := k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("When reconciling without any Registry", func() {
		It("marks the MirrorConfig not ready and renders no ConfigMap", func() {
			ctx := context.Background()

			registryList := &metalk8sv1alpha1.RegistryList{}
			Expect(k8sClient.List(ctx, registryList)).To(Succeed())
			if len(registryList.Items) > 0 {
				Skip("a Registry exists in the shared envtest, skipping the no-Registry case")
			}

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mirrorconfig-unit"}}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: ns.Name}, &corev1.Namespace{}); err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			}

			mirrorConfig := &metalk8sv1alpha1.MirrorConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "team-mirror", Namespace: ns.Name},
				Spec: metalk8sv1alpha1.MirrorConfigSpec{
					Registries: []metalk8sv1alpha1.MirrorRegistry{{Prefix: "docker.io"}},
				},
			}
			Expect(k8sClient.Create(ctx, mirrorConfig)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, mirrorConfig) })

			reconciler := &MirrorConfigReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			updated := &metalk8sv1alpha1.MirrorConfig{}
			// k8sClient is the manager's cached client: reconcile until the cache
			// has caught up with the freshly created MirrorConfig.
			Eventually(func(g Gomega) {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace},
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, updated)).To(Succeed())
				g.Expect(updated.Status.Conditions).To(HaveLen(1))
			}, 10*time.Second, time.Second).Should(Succeed())

			Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(updated.Status.Conditions[0].Reason).To(Equal("RegistryNotReady"))

			err := k8sClient.Get(ctx, types.NamespacedName{Name: mirrorConfig.Name, Namespace: mirrorConfig.Namespace}, &corev1.ConfigMap{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})
