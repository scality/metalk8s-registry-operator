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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/test/e2e/helpers"
)

// serviceMonitorAbsenceWindow is how long the RNA ServiceMonitor must stay
// absent once the Registry has been reconciled.
const serviceMonitorAbsenceWindow = 30 * time.Second

// Registry monitoring Describe: the RNA ServiceMonitor is only deployed
// once .spec.monitoring.enabled is true.
var _ = Describe("Registry monitoring", Ordered, func() {
	var workers []string

	BeforeAll(func() {
		setupCleanRegistry(&workers)
	})

	AfterAll(func() {
		cleanupRegistryState(workers, "e2e-")
	})

	It("does not deploy the RNA ServiceMonitor when spec.monitoring is omitted", func() {
		assertNoRNAServiceMonitor()
	})

	It("does not deploy the RNA ServiceMonitor when spec.monitoring.enabled is false", func() {
		Expect(helpers.SetRegistryMonitoring(ctx, k8sClient, &metalk8sv1alpha1.MonitoringSpec{
			Enabled:          false,
			PrometheusLabels: map[string]string{"release": helpers.PrometheusReleaseLabel},
		})).To(Succeed())

		assertNoRNAServiceMonitor()
	})

	It("deploys the RNA ServiceMonitor when spec.monitoring.enabled is true", func() {
		Expect(helpers.SetRegistryMonitoring(ctx, k8sClient, &metalk8sv1alpha1.MonitoringSpec{
			Enabled:          true,
			PrometheusLabels: map[string]string{"release": helpers.PrometheusReleaseLabel},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())

			sm := &monitoringv1.ServiceMonitor{}
			g.Expect(k8sClient.Get(ctx, rnaServiceMonitorKey(), sm)).To(Succeed())
			g.Expect(sm.Labels).To(HaveKeyWithValue("release", helpers.PrometheusReleaseLabel))
			owner := metav1.GetControllerOf(sm)
			g.Expect(owner).NotTo(BeNil(), "ServiceMonitor must be controlled by the Registry")
			g.Expect(owner.UID).To(Equal(reg.UID))
		}, registryReadyTimeout, statusPoll).Should(Succeed())
	})
})

// assertNoRNAServiceMonitor waits for the Registry's current generation to
// be reconciled, then checks the RNA ServiceMonitor stays absent.
func assertNoRNAServiceMonitor() {
	By("waiting for the Registry's current generation to be reconciled")
	Eventually(func(g Gomega) {
		reg, err := helpers.GetRegistry(ctx, k8sClient)
		g.Expect(err).NotTo(HaveOccurred())
		ready := apimeta.FindStatusCondition(reg.Status.Conditions, "Ready")
		g.Expect(ready).NotTo(BeNil())
		g.Expect(ready.ObservedGeneration).To(Equal(reg.Generation))
	}, registryReadyTimeout, statusPoll).Should(Succeed())

	By("checking the RNA ServiceMonitor is not deployed")
	Consistently(func(g Gomega) {
		err := k8sClient.Get(ctx, rnaServiceMonitorKey(), &monitoringv1.ServiceMonitor{})
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got: %v", err)
	}, serviceMonitorAbsenceWindow, statusPoll).Should(Succeed())
}

func rnaServiceMonitorKey() client.ObjectKey {
	return client.ObjectKey{Namespace: helpers.RegistryNamespace, Name: helpers.RNAServiceMonitorName}
}
