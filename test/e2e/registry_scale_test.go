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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/metalk8s-registry-operator/test/e2e/helpers"
)

// Registry scale-up Describe: exercises the operator's response to
// registry-role labels being added one by one, from an empty Registry all
// the way to a 3-node deployment. Deliberately does not touch
// SolutionArchives so failures in this Describe pinpoint pure
// scale-up/scale-down bugs.
var _ = Describe("Registry scale-up", Ordered, func() {
	var workers []string

	BeforeAll(func() {
		setupCleanRegistry(&workers)
	})

	AfterAll(func() {
		cleanupRegistryState(workers, "e2e-")
	})

	It("keeps the Registry empty until a node is labeled", func() {
		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(BeZero(), "no node should be selected before labelling")
			g.Expect(sum.SelectedNodes).To(BeEmpty())
		}, 30*time.Second, statusPoll).Should(Succeed())
	})

	It("deploys the registry stack on the first labeled node", func() {
		By(fmt.Sprintf("labelling %s with %s", workers[0], helpers.RegistryRoleLabel))
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[0])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(1))
			g.Expect(sum.ReadyServerReplicas).To(Equal(1))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(1))
			g.Expect(sum.Ready).To(BeTrue())
			g.Expect(sum.Available).To(BeTrue())
			g.Expect(sum.MirrorSyncReady).To(BeTrue())
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0]))
		}, registryReadyTimeout, statusPoll).Should(Succeed())
	})

	It("extends the Registry to a second node", func() {
		By(fmt.Sprintf("labelling %s", workers[1]))
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[1])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(2))
			g.Expect(sum.ReadyServerReplicas).To(Equal(2))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(2))
			g.Expect(sum.Ready).To(BeTrue())
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0], workers[1]))
		}, registryScaleTimeout, statusPoll).Should(Succeed())
	})

	It("extends the Registry to a third node", func() {
		By(fmt.Sprintf("labelling %s", workers[2]))
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[2])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(3))
			g.Expect(sum.ReadyServerReplicas).To(Equal(3))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(3))
			g.Expect(sum.Ready).To(BeTrue())
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0], workers[1], workers[2]))
		}, registryScaleTimeout, statusPoll).Should(Succeed())
	})
})
