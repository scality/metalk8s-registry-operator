# MetalK8s Registry Operator

A Kubernetes operator that manages the registry infrastructure used to distribute Solution Archives (ISO images) across the cluster.

## Overview

The operator reconciles two Custom Resources:

- **Registry** – Deploys and manages the registry stack: a **registry server** (API for listing and serving solutions) and **registry node agents** (StatefulSet that stores and serves ISO files on each node). It handles TLS (via cert-manager), mTLS for agent authentication, and placement (node selector).
- **SolutionArchive** – Represents a Solution Archive (ISO) and tracks its replication and serving status across registry node agents.

## Prerequisites

- Kubernetes cluster
- [cert-manager](https://cert-manager.io/) for TLS certificate issuance

## Quick Start

### Install the operator

```bash
# Install CRDs and deploy the operator (adjust IMG as needed)
make deploy IMG=ghcr.io/scality/metalk8s-registry-operator:latest
```

### Create a Registry

1. Create the required **Issuer** or **ClusterIssuer** and (for agent mTLS) the CA secret. See [CONTRIBUTING.md](CONTRIBUTING.md#generating-clusterissuerissuer-for-tls-certificate-generation) for examples.

2. Apply a sample Registry:

```bash
kubectl apply -f config/samples/metalk8s_v1alpha1_registry.yaml
```

3. Check status:

```bash
kubectl get registry
kubectl describe registry registry-sample
```

### Create a SolutionArchive

1. Apply a sample SolutionArchive (or create your own with `spec.name`, `spec.version`, and optional `spec.validation.checksum`):

```bash
kubectl apply -f config/samples/metalk8s_v1alpha1_solutionarchive.yaml
```

2. Check status:

```bash
kubectl get solutionarchive
kubectl describe solutionarchive metalk8s-1.25.5
```

## Configuration

### `Registry` Custom Resource

The spec includes:

| Field | Description |
|-------|-------------|
| `spec.namespace` | Namespace where registry resources are deployed (default: `metalk8s-registry-system`) |
| `spec.archivesPath` | Host path for storing ISO files (default: `/srv/scality/metalk8s/archives`) |
| `spec.solutionsPath` | Host path for mounting solutions (default: `/srv/scality/metalk8s/solutions`) |
| `spec.logLevel` | Log level |
| `spec.nodeSelector` | Node selector for scheduling registry node agents |
| `spec.server` | Registry server image and TLS certificate issuer reference |
| `spec.agent` | Registry node agent image, TLS issuer, and mTLS authentication (CA secret ref) |
| `spec.mirrorPropagation` | Containerd mirror config propagation: mirror ConfigMap generation and its sync DaemonSet. Fields: `enabled` (default true), `image` (file-reflector), `containerdConfigPath` (default `/etc/containerd/certs.d`), `nodeSelector`, `tolerations`, `ignorePaths` |

See [config/samples/metalk8s_v1alpha1_registry.yaml](config/samples/metalk8s_v1alpha1_registry.yaml) for a full example.

**Status**

The Status includes:

| Field | Description (Custom Column) |
| ----- | --------------------------- |
| `status.replicas` | Number of matching nodes (REPLICAS) |
| `status.selectedNodes` | Names of matching nodes (SELECTED NODES) |
| `status.clusterIP` | ClusterIP at which the registry is reachable, load-balanced across Registry-Server replicas (CLUSTERIP) |
| `status.nodeIPs` | Node IPs at which the registry is reachable directly on each selected node |
| `status.available` | All resources (StatefulSets, Certificates, ...) are created (no matter their status) (AVAILABLE) |
| `status.agentAvailable` | True, when all expected Registry-Node-Agent are available (no matter their status)  |
| `status.serverAvailable` | True, when all expected Registry-Server are available (no matter their status) |
| `status.ready` | All resources (StatefulSets, Certificates, ...) are ready (READY) |
| `status.agentReady` | True, when all expected Registry-Node-Agent are ready |
| `status.serverReady` | True, when all expected Registry-Server are ready |
| `status.readyAgentReplicas` | Number of ready Registry-Node-Agent (AGENT REPLICAS) |
| `status.readyServerReplicas` | Number of ready Registry-Server (SERVER REPLICAS) |
| `status.statusPerNode` | Same information as below, sorted by `Nodes` |

Example (Conditions omited for clarity)
```yaml
status:
  agentAvailable: true
  agentReady: true
  available: true
  clusterIP: 10.43.0.10
  nodeIPs:
  - 10.0.0.1
  - 10.0.0.2
  ready: true
  readyAgentReplicas: 2
  readyServerReplicas: 2
  replicas: 2
  selectedNodes:
  - k3d-k3d-agent-0
  - k3d-k3d-agent-1
  serverAvailable: true
  serverReady: true
  statusPerNode:
    k3d-k3d-agent-0:
      agent:
        available: true
        ready: true
      server:
        available: true
        ready: true
    k3d-k3d-agent-1:
      agent:
        available: true
        ready: true
      server:
        available: true
        ready: true
```

### `SolutionArchive` Custom Resource

The spec includes:

| Field | Description |
|-------|-------------|
| `spec.name` | Solution name (e.g. `metalk8s`) |
| `spec.version` | Solution version (e.g. `1.25.5`) |
| `spec.validation.checksum` | Optional checksum (type and value) to validate the archive |

See [config/samples/metalk8s_v1alpha1_solutionarchive.yaml](config/samples/metalk8s_v1alpha1_solutionarchive.yaml) for examples.

**Status**

The Status includes:

| Field | Description (Custom Column) |
| ----- | --------------------------- |
| `status.served` | True, when, at least, one NodeSolutionArchive in Served status (SERVED) |
| `status.replicated` | True, when all NodeSolutionArchive in Served status (REPLICATED) |
| `status.targetReplicas` | Expected number of NodeSolutionArchive in Served status (TARGET) |
| `status.servedReplicas` | Number of NodeSolutionArchive in Served status (REPLICAS) |
| `status.nodeSolutionArchives` | Names of `NodeSolutionArchives` (NODE SOLUTION ARCHIVES) |
| `status.statusPerNodeSolutionArchive` | `Served` status sorted by `NodeSolutionArchives` |

Example (Conditions omited for clarity)
```yaml
status:
  nodeSolutionArchives:
  - metalk8s-1.25.4-k3d-k3d-agent-0
  - metalk8s-1.25.4-k3d-k3d-agent-1
  replicated: true
  served: true
  servedReplicas: 2
  statusPerNodeSolutionArchive:
    metalk8s-1.25.4-k3d-k3d-agent-0:
      served: true
    metalk8s-1.25.4-k3d-k3d-agent-1:
      served: true
  targetReplicas: 2
```

## Building

```bash
make build
make docker-build IMG=ghcr.io/scality/metalk8s-registry-operator:latest
```

## Development and testing

```bash
make test
make test-e2e
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, certificate generation, and guidelines.

## License

See [LICENSE](LICENSE) for details.
