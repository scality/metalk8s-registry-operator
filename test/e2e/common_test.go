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

// Shared setup / teardown / assertion helpers used by every Describe in
// this package. Kept in a dedicated file so the flow of an individual
// spec file remains easy to follow.

package e2e

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	"github.com/scality/metalk8s-registry-operator/test/e2e/helpers"
)

// Timing budgets used by every spec. The upper bounds account for the
// slowest reasonable happy path we have observed (image pull time on cold
// nodes, big-archive uploads with 1 MiB chunks, RNA pod scheduling).
const (
	registryReadyTimeout     = 5 * time.Minute
	registryScaleTimeout     = 5 * time.Minute
	registryDeleteTimeout    = 5 * time.Minute
	replicationTimeout       = 10 * time.Minute
	bigReplicationTimeout    = 20 * time.Minute
	uploadSmallTimeout       = 2 * time.Minute
	uploadBigTimeout         = 15 * time.Minute
	pullTimeout              = 3 * time.Minute
	saDeletionTimeout        = 5 * time.Minute
	statusPoll               = 3 * time.Second
	taintedNodeEvictionTaint = "e2e.metalk8s.scality.com/evict"
	pullerNamespace          = "default"
	sharedPullerPrefix       = "e2e-puller"
)

// setupCleanRegistry resets the shared cluster state to a known-good
// starting point: worker list populated, CA/Issuer present, no leftover
// Registry, no registry-role labels, no stale per-node TLS secrets — then
// creates a fresh Registry CR. Intended to be called from BeforeAll in
// every Ordered Describe that needs a clean canvas.
func setupCleanRegistry(workers *[]string) {
	By("listing worker nodes")
	var err error
	*workers, err = helpers.ListWorkerNodes(ctx, k8sClient)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(*workers)).To(BeNumerically(">=", 4),
		"at least 4 worker nodes are required (3 registry + 1 puller); got %d: %v", len(*workers), *workers)

	By("ensuring the CA + Issuer exist so any pending Registry finalizer can succeed")
	_, _, err = helpers.EnsureCA(ctx, k8sClient)
	Expect(err).NotTo(HaveOccurred())

	By("waiting for any previous Registry teardown to complete")
	waitForRegistryGone(2 * time.Minute)

	By("clearing every registry role label so the Registry starts empty")
	Expect(helpers.ResetRegistryLabels(ctx, k8sClient)).To(Succeed())

	By("wiping stale per-node TLS secrets from any previous run")
	Expect(helpers.DeletePerNodeTLSSecrets(ctx, k8sClient)).To(Succeed(),
		"per-node TLS secrets from a previous run must be gone so cert-manager re-issues")

	By("creating the Registry custom resource")
	_, err = helpers.EnsureRegistry(ctx, k8sClient)
	Expect(err).NotTo(HaveOccurred())
}

// labelAndWaitForRegistry labels the first `count` workers with the
// registry role and blocks until the Registry converges to `count`/`count`
// ready replicas. Intended for BeforeAll blocks that need the Registry
// already scaled to a specific size before the spec body runs — for the
// scale-up transitions that are themselves the subject of a test, prefer
// asserting the transition inline in the spec.
func labelAndWaitForRegistry(workers []string, count int) {
	labeled := workers[:count]
	By(fmt.Sprintf("labelling %d registry node(s): %v", count, labeled))
	for _, n := range labeled {
		Expect(helpers.LabelAsRegistry(ctx, k8sClient, n)).To(Succeed())
	}
	Eventually(func(g Gomega) {
		reg, err := helpers.GetRegistry(ctx, k8sClient)
		g.Expect(err).NotTo(HaveOccurred())
		sum := helpers.Summarize(reg)
		g.Expect(sum.Replicas).To(Equal(count))
		g.Expect(sum.ReadyServerReplicas).To(Equal(count))
		g.Expect(sum.ReadyAgentReplicas).To(Equal(count))
		g.Expect(sum.Ready).To(BeTrue())
		g.Expect(sum.SelectedNodes).To(ConsistOf(labeled))
	}, registryScaleTimeout, statusPoll).Should(Succeed(),
		"Registry should reach %d/%d ready replicas after labelling %v", count, count, labeled)
}

// cleanupRegistryState is the shared teardown routine used by every Ordered
// Describe in this package. Ordering matters: SolutionArchives must be
// fully deleted (which triggers RNA-side disk cleanup via the finalizer)
// BEFORE we unlabel nodes, otherwise the RNA StatefulSet is torn down
// while the NSA finalizer is still running and the NSA is left stuck.
// If a SolutionArchive gets stuck (e.g. its NSAs are blocked on an RNA
// pod that is no longer scheduled), we force-strip the finalizers on
// every remaining NSA so the next Describe can start from a clean slate.
func cleanupRegistryState(workers []string, saPrefix string) {
	By(fmt.Sprintf("removing SolutionArchives with prefix %q", saPrefix))
	saList := &metalk8sv1alpha1.SolutionArchiveList{}
	if err := k8sClient.List(ctx, saList); err == nil {
		for i := range saList.Items {
			sa := &saList.Items[i]
			if !strings.HasPrefix(sa.Name, saPrefix) {
				continue
			}
			_ = k8sClient.Delete(ctx, sa)
		}
	}

	By("waiting for NodeSolutionArchives to be gone (or force-removing stuck ones)")
	// SolutionArchive has no controller-managed finalizer, so `Delete` on
	// the SA returns immediately (background cascade) — the SA disappears
	// while its child NSAs are still finalising. NSAs however carry a
	// finalizer that only completes once the RNA pod has run its disk
	// cleanup; if the RNA is Pending/unschedulable (e.g. because a prior
	// test tainted the node or unlabeled it) the NSA hangs indefinitely
	// and, transitively, blocks the Registry CR deletion via the
	// deleteUnusedRNAResourcesByNode path in the Registry reconciler.
	// Poll NSAs (not SAs) so we actually observe and force-strip the
	// stragglers.
	Eventually(func(g Gomega) {
		nsas := &rnav1alpha1.NodeSolutionArchiveList{}
		g.Expect(k8sClient.List(ctx, nsas)).To(Succeed())
		remaining := 0
		for i := range nsas.Items {
			if strings.HasPrefix(nsas.Items[i].Name, saPrefix) {
				remaining++
			}
		}
		if remaining > 0 {
			// Escape hatch: strip finalizers off every stuck NSA so the
			// operator can complete Registry cleanup.
			_, _ = helpers.ForceCleanupNodeArchives(ctx, k8sClient, saPrefix)
		}
		g.Expect(remaining).To(Equal(0),
			"%d NodeSolutionArchive(s) still present", remaining)
	}, saDeletionTimeout, statusPoll).Should(Succeed(),
		"forcing NSA finalizer removal because NSAs are stuck")

	By("deleting every puller pod created by this suite")
	pods := &corev1.PodList{}
	if err := listPullerPods(pods); err == nil {
		for i := range pods.Items {
			_ = k8sClient.Delete(ctx, &pods.Items[i])
		}
	}

	By("dropping every registry-role label and e2e taint")
	for _, n := range workers {
		_ = helpers.UnlabelAsRegistry(ctx, k8sClient, n)
		_ = helpers.Untaint(ctx, k8sClient, n, taintedNodeEvictionTaint, corev1.TaintEffectNoExecute)
	}

	By("deleting the Registry CR")
	_ = helpers.DeleteRegistry(ctx, k8sClient)

	By("waiting for the Registry CR to be fully gone")
	// The Registry finalizer patches the CR through the validating webhook,
	// which requires .spec.agent.authentication.mtls.CASecretRef and both
	// certificateIssuerRefs to still resolve. We therefore MUST wait for
	// the Registry to be gone BEFORE deleting the CA Secret + Issuer,
	// otherwise the operator gets stuck in an infinite webhook-denied loop
	// on every finalizer removal attempt.
	waitForRegistryGone(registryDeleteTimeout)

	By("deleting the CA Secret + Issuer used by the Registry")
	Expect(helpers.DeleteCAArtifacts(ctx, k8sClient)).To(Succeed())

	By("wiping per-node TLS secrets so the next Registry re-issues certs")
	// cert-manager does NOT garbage-collect the Secrets it materialises when
	// the parent Certificate resource is deleted. If we leave them behind,
	// the next Registry may schedule its RS/RNA pods before cert-manager
	// finishes re-issuing certificates with the new ClusterIP as a SAN,
	// causing intermittent TLS SAN mismatch failures on containerd pulls.
	Expect(helpers.DeletePerNodeTLSSecrets(ctx, k8sClient)).To(Succeed(),
		"per-node TLS secrets must be deleted so cert-manager re-issues fresh certs")
}

// waitForRegistryGone polls until the Registry CR is fully gone. If it
// remains after the timeout, the operator's finalizer is likely stuck —
// most commonly because a previous cleanup deleted the CA Secret / Issuer
// too early and the validating webhook now denies every finalizer patch.
// As an escape hatch, force-strip the Registry finalizers and re-poll.
func waitForRegistryGone(timeout time.Duration) {
	registryGone := func() bool {
		_, err := helpers.GetRegistry(ctx, k8sClient)
		return apierrors.IsNotFound(err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if registryGone() {
			return
		}
		time.Sleep(statusPoll)
	}

	By("Registry stuck — force-stripping Registry finalizers")
	Expect(helpers.ForceRemoveRegistryFinalizers(ctx, k8sClient)).To(Succeed())
	Eventually(registryGone, 30*time.Second, statusPoll).Should(BeTrue(),
		"Registry CR should be gone after force-removing finalizers")
}

// pullerNodeAmong returns a worker that is NOT one of the first three
// (which the tests use for the Registry). Every Describe asserts >=4
// workers in BeforeAll, so this always resolves.
func pullerNodeAmong(workers []string) string {
	if len(workers) < 4 {
		Fail(fmt.Sprintf("need at least 4 worker nodes; got %d (%v)", len(workers), workers))
	}
	return workers[len(workers)-1]
}

// uploadFixture wraps EnsureArchive + UploadArchiveISO in a single
// deterministic step. It fails the spec (rather than returning) on error.
func uploadFixture(fx helpers.Fixture, targetNode string, timeout time.Duration) {
	spec := fx.ArchiveSpec()
	_, err := helpers.EnsureArchive(ctx, k8sClient, spec)
	Expect(err).NotTo(HaveOccurred(), "creating SolutionArchive %s", fx.SolutionName)

	uploadCtx, cancelUpload := ctxWithTimeout(timeout)
	defer cancelUpload()
	Expect(helpers.UploadArchiveISO(uploadCtx, k8sClient, spec, targetNode)).To(Succeed(),
		"uploading %s to %s", fx.ISOPath(), targetNode)
}

// waitForArchive polls the given fixture's SolutionArchive until
// Replicated=true, Served=true, and ServedReplicas>=minReplicas.
func waitForArchive(fx helpers.Fixture, minReplicas int, timeout time.Duration) {
	waitForArchiveName(fmt.Sprintf("%s-%s", fx.SolutionName, fx.SolutionVersion), minReplicas, timeout)
}

// waitForArchiveName is the same as waitForArchive but starts from the
// derived CR name (used when iterating over a listed set).
func waitForArchiveName(name string, minReplicas int, timeout time.Duration) {
	Eventually(func(g Gomega) {
		sa := &metalk8sv1alpha1.SolutionArchive{}
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, sa)).To(Succeed())
		sum := helpers.SummarizeArchive(sa)
		g.Expect(sum.Served).To(BeTrue(), "SA %s not yet Served", name)
		g.Expect(sum.Replicated).To(BeTrue(), "SA %s not yet Replicated", name)
		g.Expect(sum.ServedReplicas).To(BeNumerically(">=", minReplicas),
			"SA %s ServedReplicas=%d, want >= %d", name, sum.ServedReplicas, minReplicas)
	}, timeout, statusPoll).Should(Succeed(),
		"SolutionArchive %s should reach Replicated=true with ServedReplicas>=%d", name, minReplicas)
}

// assertPullSucceeds launches one puller Pod per image ref in parallel,
// waits for each to report imageID populated (== pull succeeded), and
// deletes the pods. Every puller uses a unique name derived from
// tag+index so specs can be re-run without collision. Concurrent pulls
// exercise containerd's own parallel-pull path; if any one pod fails the
// spec fails via GinkgoRecover.
func assertPullSucceeds(refs []string, pullerNode, tag string) {
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Add(1)
		name := fmt.Sprintf("%s-%s-%d", sharedPullerPrefix, sanitisePodName(tag), i)
		go func(ref, name string) {
			defer GinkgoRecover()
			defer wg.Done()
			pullOne(ref, pullerNode, name, true)
		}(ref, name)
	}
	wg.Wait()
}

// assertPullFails is the inverse: it expects the pod to reach a
// container-status waiting reason matched by IsImagePullErrorReason.
func assertPullFails(ref, pullerNode, tag string) {
	name := fmt.Sprintf("%s-%s-fail", sharedPullerPrefix, sanitisePodName(tag))
	pullOne(ref, pullerNode, name, false)
}

func pullOne(ref, pullerNode, podName string, expectSuccess bool) {
	// Clean up any leftover puller from a previous run.
	_ = helpers.DeletePullerPod(ctx, k8sClient, podName, pullerNamespace)
	Eventually(func() bool {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: pullerNamespace}, pod)
		return apierrors.IsNotFound(err)
	}, 30*time.Second, time.Second).Should(BeTrue(), "cleanup of %s should complete", podName)

	_, err := helpers.StartPullerPod(ctx, k8sClient, helpers.PullerPodSpec{
		Name:      podName,
		Namespace: pullerNamespace,
		NodeName:  pullerNode,
		Image:     ref,
	})
	Expect(err).NotTo(HaveOccurred(), "creating puller pod %s", podName)

	defer func() {
		_ = helpers.DeletePullerPod(ctx, k8sClient, podName, pullerNamespace)
	}()

	Eventually(func(g Gomega) {
		res, err := helpers.PullStatus(ctx, k8sClient, podName, pullerNamespace)
		g.Expect(err).NotTo(HaveOccurred())
		if expectSuccess {
			g.Expect(res.Success).To(BeTrue(),
				"pod %s should pull %s (last reason=%s, message=%s)",
				podName, ref, res.Reason, res.Message)
			return
		}
		g.Expect(helpers.IsImagePullErrorReason(res.Reason)).To(BeTrue(),
			"pod %s should reach an image-pull error state for %s; observed reason=%s message=%s",
			podName, ref, res.Reason, res.Message)
	}, pullTimeout, statusPoll).Should(Succeed())
}

// sanitisePodName strips characters that are illegal in DNS-1123 pod names.
func sanitisePodName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// ctxWithTimeout produces a child context bounded to `d` beyond the shared
// suite-level context. The returned cancel MUST be called even on success
// (Go vet enforces this).
func ctxWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

// listPullerPods lists every puller Pod created by any spec in this suite.
// The label filter matches whatever StartPullerPod stamps on the Pod at
// creation time.
func listPullerPods(dst *corev1.PodList) error {
	return k8sClient.List(ctx, dst, client.MatchingLabels{
		"e2e.metalk8s.scality.com/role": "puller",
	})
}
