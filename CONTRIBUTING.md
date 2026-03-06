# Contributing to MetalK8s Registry Operator

This document provides guidelines for contributing to the MetalK8s Registry Operator project.

## Repository Structure

The project is scaffolded with [Kubebuilder](https://book.kubebuilder.io/) and follows the standard Operator SDK layout, with a clear separation between API types, controllers, webhooks, and tests:

```
├── api/                   # Kubernetes API definitions (CRDs)
│   └── v1alpha1/          # API version v1alpha1 types
├── cmd/                   # Application entry point
│   └── config/            # Environment configuration
├── config/                # Kubernetes manifests and kustomize overlays
├── hack/                  # Build and development scripts
├── internal/              # Internal packages (not importable)
│   ├── controller/        # Kubernetes controller logic
│   ├── utils/             # Shared internal utilities
│   └── webhook/           # Admission webhook handlers
│       └── v1alpha1/      # Webhook handlers for v1alpha1 API
└── test/                  # Test suites
    ├── e2e/               # End-to-end tests
    ├── integration/       # Integration tests
    └── utils/             # Test utilities
```

## Development Setup

### Prerequisites

- Go 1.25.0 or later
- Docker for containerization
- Kubernetes cluster
- kubectl for Kubernetes integration
- Git for version control
- cert-manager for kubernetes deployment

## Building

```bash
# Build the binary
make build

# Build the Docker image
make docker-build IMG=registry.localhost:5000/registry-operator:0.1.0
```

### Generating Secret for mTLS Authentication
This secret contains the CA that sign Client `Certificate` used to upload Solution Archives ISO files. They must exist prior to applying `Registry` Custom Resource.

```bash
kubectl create ns my-namespace
kubectl create secret generic registry-agent-mtls-ca \
    -n my-namespace \
    --from-file /certs/client/external/ca.crt
```

The secret must conform to the field `Registry.spec.agent.authentication.mtls.caSecretRef`, the secret key must be named `ca.crt`

### Generating ClusterIssuer/Issuer for TLS Certificate generation
Both Registry-Node-Agent and Registry-Server establish TLS connection with their client, this requires a `Certificate` build with cert-manager `Issuer`/`ClusterIssuer`.  
In the following example, we consider:
* `ca.crt` and `ca.key` already exist
* using the Same `ClusterIssuer`/`Issuer` for both Registry-Node-Agent and Registry-Server

**For Issuer**
```bash
kubectl create secret tls ca-for-issuer \
   --cert=/certs/server/external/ca.crt \
   --key=/certs/server/external/ca.key \
   --namespace=my-namespace
```
```bash
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: ca-issuer
  namespace: my-namespace
spec:
  ca:
    secretName: ca-for-issuer
EOF
```

**For ClusterIssuer**
```bash
kubectl create secret tls ca-for-cluster-issuer \
   --cert=/certs/server/external/ca.crt \
   --key=/certs/server/external/ca.key \
   --namespace=cert-manager
```
```bash
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ca-cluster-issuer
spec:
  ca:
    secretName: ca-for-cluster-issuer
EOF
```

`ClusterIssuer` or `Issuer` name and kind must conform to `Registry.spec.server.certificateIssuerRef` and `Registry.spec.agent.certificateIssuerRef`
