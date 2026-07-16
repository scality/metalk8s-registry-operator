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
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/test/e2e/helpers"
)

// e2eRandomSeed makes the "distribute across nodes" step reproducible when
// GINKGO_SEED is set — Ginkgo already surfaces `GinkgoRandomSeed()` for
// this purpose.
func e2eRandomSeed() *rand.Rand {
	return rand.New(rand.NewSource(GinkgoRandomSeed()))
}

// Solution archive replication Describe: exercises upload of the four
// fixture shapes (small single-image, big single-image, small multi-image,
// bulk 10+2) into a two-node Registry and validates that each archive
// replicates and can be pulled by containerd. The final spec extends the
// Registry to a third node and asserts that (a) pulls keep working while
// replication is in flight and (b) every previously-uploaded archive
// eventually replicates to the new node too.
var _ = Describe("Solution archive replication", Ordered, func() {
	var workers []string

	BeforeAll(func() {
		setupCleanRegistry(&workers)
		labelAndWaitForRegistry(workers, 2)
	})

	AfterAll(func() {
		cleanupRegistryState(workers, "e2e-")
	})

	It("replicates a small single-image archive to both registry nodes", func() {
		fx := helpers.SmallSingle
		By(fmt.Sprintf("uploading %s to %s", fx.ISOPath(), workers[0]))
		uploadFixture(fx, workers[0], uploadSmallTimeout)

		waitForArchive(fx, 2, replicationTimeout)
		assertPullSucceeds(fx.PullRefs(), pullerNodeAmong(workers), "small-single")
	})

	It("replicates a big archive to both registry nodes", func() {
		fx := helpers.Big
		By(fmt.Sprintf("uploading %s (BIG_ISO_SIZE) to %s", fx.ISOPath(), workers[0]))
		uploadFixture(fx, workers[0], uploadBigTimeout)

		waitForArchive(fx, 2, bigReplicationTimeout)
		assertPullSucceeds(fx.PullRefs(), pullerNodeAmong(workers), "big-image")
	})

	It("replicates a multi-image archive to both registry nodes", func() {
		fx := helpers.SmallMulti
		By(fmt.Sprintf("uploading %s to %s", fx.ISOPath(), workers[1]))
		uploadFixture(fx, workers[1], uploadSmallTimeout)

		waitForArchive(fx, 2, replicationTimeout)
		assertPullSucceeds(fx.PullRefs(), pullerNodeAmong(workers), "small-multi")
	})

	It("distributes 10 small + 2 big archives across the labeled nodes", func() {
		labeled := []string{workers[0], workers[1]}
		bulk := helpers.BulkFixtures()
		Expect(bulk).NotTo(BeEmpty(), "BulkFixtures should return at least the 12 expected entries")

		rng := e2eRandomSeed()
		By(fmt.Sprintf("uploading %d bulk fixtures to random registry nodes", len(bulk)))
		for _, fx := range bulk {
			target := labeled[rng.Intn(len(labeled))]
			timeout := uploadSmallTimeout
			if strings.Contains(fx.SolutionName, "bulk-big-") {
				timeout = uploadBigTimeout
			}
			By(fmt.Sprintf("uploading %s → %s", fx.SolutionName, target))
			uploadFixture(fx, target, timeout)
		}

		By("waiting for every bulk archive to fully replicate")
		for _, fx := range bulk {
			timeout := replicationTimeout
			if strings.Contains(fx.SolutionName, "bulk-big-") {
				timeout = bigReplicationTimeout
			}
			waitForArchive(fx, 2, timeout)
		}

		By("pulling every bulk image from the puller node")
		allRefs := make([]string, 0, len(bulk))
		for _, fx := range bulk {
			allRefs = append(allRefs, fx.PullRefs()...)
		}
		assertPullSucceeds(allRefs, pullerNodeAmong(workers), "bulk")
	})

	It("extends to a third registry node and syncs while pulls keep working", func() {
		By(fmt.Sprintf("labelling %s", workers[2]))
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, workers[2])).To(Succeed())

		By("issuing a pull immediately (before waiting for replicated=true on the new node)")
		// The mirror routes to a peer that already serves the image, so pulls
		// should keep working even while replication is in-flight on node[2].
		assertPullSucceeds(helpers.SmallSingle.PullRefs(), pullerNodeAmong(workers), "during-sync")

		By("waiting for the Registry itself to reach 3/3")
		Eventually(func(g Gomega) {
			reg, err := helpers.GetRegistry(ctx, k8sClient)
			g.Expect(err).NotTo(HaveOccurred())
			sum := helpers.Summarize(reg)
			g.Expect(sum.Replicas).To(Equal(3))
			g.Expect(sum.ReadyServerReplicas).To(Equal(3))
			g.Expect(sum.ReadyAgentReplicas).To(Equal(3))
			g.Expect(sum.Ready).To(BeTrue())
		}, registryScaleTimeout, statusPoll).Should(Succeed())

		By("waiting for every SolutionArchive to fully replicate to all three nodes")
		saList := &metalk8sv1alpha1.SolutionArchiveList{}
		Expect(k8sClient.List(ctx, saList)).To(Succeed())
		for i := range saList.Items {
			sa := &saList.Items[i]
			timeout := replicationTimeout
			if strings.Contains(sa.Name, "big") {
				timeout = bigReplicationTimeout
			}
			waitForArchiveName(sa.Name, 3, timeout)
		}
	})
})
