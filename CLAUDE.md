# metalk8s-registry-operator

This is a **Go Kubernetes operator** that manages registry infrastructure for distributing Solution Archives (ISO images) across MetalK8s clusters. It contains:

- CRD definitions for `Registry` and `SolutionArchive` resources (`api/v1alpha1/`)
- Reconciliation controllers (`internal/controller/`)
- Validation/mutation webhooks (`internal/webhook/v1alpha1/`)
- Helm charts for deployment (`charts/`)
- Kustomize manifests (`config/`)
- Integration tests using envtest (`test/integration/`)
- End-to-end tests against a live cluster (`test/e2e/`)

## Tech stack

- Go 1.25 with controller-runtime (kubebuilder-style operator)
- cert-manager integration for TLS/mTLS
- Ginkgo v2 + Gomega for testing
- golangci-lint v2 for linting (strict config in `.golangci.yml`)
- Git-based Scality internal dep: `github.com/scality/metalk8s-registry-node-agent`

## Common commands

- `make test` — run unit and integration tests (envtest)
- `make test-e2e` — run the e2e suite against the cluster pointed to by `$KUBECONFIG`
- `make manifests` — regenerate CRD manifests
- `make generate` — regenerate deepcopy and other generated code
- `make build` — build the operator binary
- `make docker-build` — build the Docker image
