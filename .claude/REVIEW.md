# Review criteria

Read by the `/review-pr` skill (Scality agent hub) and by anyone reviewing by hand.
Flag problems only — see "What not to flag" at the end.

## What this repo is

A Go Kubernetes operator that manages the registry infrastructure distributing
Solution Archives across the cluster. It reconciles two custom resources:
`Registry` (registry server plus the node-agent StatefulSet, TLS through
cert-manager, mTLS for agent authentication, placement) and `SolutionArchive`
(replication and serving status of an archive across the node agents). It also
ships the Helm chart used to deploy that stack.

## Criteria

| Area | What to check |
|------|---------------|
| Error wrapping | Use `fmt.Errorf("...: %w", err)`, not `%v`; don't swallow errors silently |
| Context propagation | Pass `context.Context` through call chains, respect cancellation |
| Goroutine leaks | Ensure goroutines have exit conditions, use `errgroup` where appropriate |
| Kubernetes operator patterns | RBAC scoping matches actual API calls, reconciler idempotency, status subresource updates, proper use of controller-runtime predicates and event filters |
| Interface compliance | Check that implementations satisfy interfaces at compile time (`var _ Interface = &Struct{}`) |
| CRD changes | Backward compatibility of API types, proper kubebuilder markers, deepcopy generation |
| cert-manager integration | Correct issuer references, certificate rotation handling, mTLS configuration consistency |
| Helm chart changes | Template syntax correctness, value defaults, upgrade path, label selector immutability |
| Kubernetes manifests | Resource limits set, security contexts defined, proper label selectors |
| Dependency pinning | Git-based deps (especially `metalk8s-registry-node-agent`) must pin to a tag, not a branch |
| Testing | New behavior has corresponding Ginkgo tests, test assertions are specific (not just `Expect(err).ToNot(HaveOccurred())` when the value matters too) |
| Security | No credentials or tokens in plain text, RBAC follows least privilege, container security contexts |
| Breaking changes | Anything that changes CRD schemas, public APIs, or Helm chart values |

## What not to flag

- Anything the linters already own: `golangci-lint` (govet, staticcheck, revive,
  errcheck, gocyclo, dupl…), `gofmt`, `goimports`.
- Generated files (`zz_generated.*`, CRD manifests) except when they are stale with
  respect to the sources changed in the same PR.
- Markdown or comment wording preferences.
- Refactors unrelated to the PR's purpose.
