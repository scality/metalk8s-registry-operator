/*
Copyright 2026 Scality.

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

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	operatorNamespace  = "metalk8s-registry-system"
	operatorDeployment = "metalk8s-registry-operator-controller-manager"
)

var expectedCRDs = []string{
	"registries.metalk8s.scality.com",
	"mirrorconfigs.metalk8s.scality.com",
	"solutionarchives.metalk8s.scality.com",
}

var _ = Describe("Operator deployment sanity", func() {
	It("registers the operator CRDs", func() {
		for _, name := range expectedCRDs {
			crd := &apiextv1.CustomResourceDefinition{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, crd)).To(Succeed())
				g.Expect(crdEstablished(crd)).To(BeTrue(),
					"CRD %s should reach the Established condition", name)
			}, 30*time.Second, 2*time.Second).Should(Succeed(),
				"CRD %s should be installed and Established", name)
		}
	})

	It("runs the controller manager deployment with all replicas available", func() {
		deploy := &appsv1.Deployment{}
		key := types.NamespacedName{Namespace: operatorNamespace, Name: operatorDeployment}

		Eventually(func() error {
			return k8sClient.Get(ctx, key, deploy)
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"deployment %s/%s should exist", operatorNamespace, operatorDeployment)

		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, key, deploy)).To(Succeed())
			g.Expect(deploy.Status.ObservedGeneration).To(BeNumerically(">=", deploy.Generation))
			g.Expect(deploy.Status.ReadyReplicas).To(Equal(desired))
			g.Expect(deploy.Status.AvailableReplicas).To(Equal(desired))
			g.Expect(deploy.Status.UnavailableReplicas).To(BeZero())
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"deployment %s/%s should have %d ready replicas", operatorNamespace, operatorDeployment, desired)
	})

	It("has at least one Ready controller-manager pod", func() {
		pods := &corev1.PodList{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.List(ctx, pods,
				client.InNamespace(operatorNamespace),
				client.MatchingLabels{"control-plane": "controller-manager"},
			)).To(Succeed())
			g.Expect(pods.Items).NotTo(BeEmpty(),
				"expected at least one controller-manager pod in %s", operatorNamespace)
			for _, pod := range pods.Items {
				g.Expect(podReady(pod)).To(BeTrue(),
					"pod %s/%s should be Ready", pod.Namespace, pod.Name)
			}
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})
})

func crdEstablished(crd *apiextv1.CustomResourceDefinition) bool {
	for _, c := range crd.Status.Conditions {
		if c.Type == apiextv1.Established && c.Status == apiextv1.ConditionTrue {
			return true
		}
	}
	return false
}

func podReady(pod corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
