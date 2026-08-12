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

// Registry node availability Describe: exercises the behaviour of an
// already-provisioned Registry when its registry nodes are made
// unavailable — first by NoExecute taint (pods evicted but label
// remains), then by dropping the label from every remaining node. Starts
// from a 3-node Registry with the small single-image archive already
// replicated to all three nodes so pull assertions are meaningful.
var _ = Describe("Registry node availability", Ordered, func() {
	var workers []string

	BeforeAll(func() {
		setupCleanRegistry(&workers)
		labelAndWaitForRegistry(workers, 3)

		By("uploading the small single-image archive as the pull probe")
		uploadFixture(helpers.SmallSingle, workers[0], uploadSmallTimeout)
		waitForArchive(helpers.SmallSingle, 3, replicationTimeout)
	})

	AfterAll(func() {
		cleanupRegistryState(workers, "e2e-")
	})

	It("keeps pulls working when a registry node is tainted with NoExecute", func() {
		By(fmt.Sprintf("tainting %s with NoExecute so its RNA/RS pods are evicted", workers[0]))
		Expect(helpers.TaintNoExecute(ctx, k8sClient, workers[0], taintedNodeEvictionTaint)).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			// State on entry: 3 nodes labelled + workers[0] just tainted
			// with NoExecute → its RS/RNA pods are evicted, leaving
			// exactly two healthy replicas on workers[1] and workers[2].
			g.Expect(sum.ReadyAgentReplicas).To(Equal(2))
			g.Expect(sum.ReadyServerReplicas).To(Equal(2))
		}, registryScaleTimeout, statusPoll).Should(Succeed())

		By("pulls still succeed because the mirror routes to healthy peers")
		assertPullSucceeds(helpers.SmallSingle.PullRefs(), pullerNodeAmong(workers), "after-taint")
	})

	It("stops serving pulls when every remaining registry node is unlabeled", func() {
		By(fmt.Sprintf("unlabelling %s and %s", workers[1], workers[2]))
		Expect(helpers.UnlabelAsRegistry(ctx, k8sClient, workers[1])).To(Succeed())
		Expect(helpers.UnlabelAsRegistry(ctx, k8sClient, workers[2])).To(Succeed())

		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			// Note: .status.available is (as of the current controller)
			// hardcoded to true whenever the reconciler completes without
			// error, so it is NOT a useful readiness signal here — we key
			// off ready-replicas instead. The follow-up pull assertion
			// carries the "not serving" contract end-to-end.
			g.Expect(sum.ReadyServerReplicas).To(BeZero())
			g.Expect(sum.ReadyAgentReplicas).To(BeZero())
		}, registryScaleTimeout, statusPoll).Should(Succeed())

		By("pulls should now fail with an image-pull error")
		assertPullFails(helpers.SmallSingle.PullRefs()[0], pullerNodeAmong(workers), "after-unlabel")
	})
})
