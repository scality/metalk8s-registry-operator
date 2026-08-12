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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/metalk8s-registry-operator/test/e2e/helpers"
)

// This Describe re-does the extension flow using a big-only archive so we
// exercise the "big data replicating while new nodes come online" path
// with a fresh Registry. Ginkgo runs Describes in file-declaration order
// (they share the same package); BeforeAll resets the Registry to a
// clean canvas and AfterAll waits for full teardown so this Describe is
// insensitive to whichever Describe ran before it.
var _ = Describe("Big archive across registry extensions", Ordered, func() {
	var workers []string

	BeforeAll(func() {
		setupCleanRegistry(&workers)
	})

	AfterAll(func() {
		cleanupRegistryState(workers, "e2e-big-extension")
	})

	It("uploads a big archive to the first registry node", func() {
		By(fmt.Sprintf("labelling %s", workers[0]))
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[0])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(1))
			g.Expect(sum.ReadyServerReplicas).To(Equal(1))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(1))
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0]))
		}, registryReadyTimeout, statusPoll).Should(Succeed())

		fx := helpers.BigExtension
		By(fmt.Sprintf("uploading %s to %s", fx.ISOPath(), workers[0]))
		uploadFixture(fx, workers[0], uploadBigTimeout)

		waitForArchive(fx, 1, bigReplicationTimeout)
		assertPullSucceeds(fx.PullRefs(), pullerNodeAmong(workers), "bigext-1node")
	})

	It("extends to a second node and replicates the big archive", func() {
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[1])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(2))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(2))
			g.Expect(sum.ReadyServerReplicas).To(Equal(2))
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0], workers[1]))
		}, registryScaleTimeout, statusPoll).Should(Succeed())

		waitForArchive(helpers.BigExtension, 2, bigReplicationTimeout)
		assertPullSucceeds(helpers.BigExtension.PullRefs(), pullerNodeAmong(workers), "bigext-2nodes")
	})

	It("extends to a third node and replicates the big archive", func() {
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[2])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(3))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(3))
			g.Expect(sum.ReadyServerReplicas).To(Equal(3))
			g.Expect(sum.SelectedNodes).To(ConsistOf(workers[0], workers[1], workers[2]))
		}, registryScaleTimeout, statusPoll).Should(Succeed())

		waitForArchive(helpers.BigExtension, 3, bigReplicationTimeout)
		assertPullSucceeds(helpers.BigExtension.PullRefs(), pullerNodeAmong(workers), "bigext-3nodes")
	})
})
