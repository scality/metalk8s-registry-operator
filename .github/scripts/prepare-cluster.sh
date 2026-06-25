#!/usr/bin/env bash
#
# Bootstraps a kubeadm cluster on hosts defined in ./ssh_config.
#
# Expects (from the calling action):
#   NODE_COUNT       e.g. "4" (1 control-plane + N-1 workers, all named node-1..node-N)
#   KUBE_VERSION     full kubeadm/kubelet version, e.g. "v1.34.9"
#   POD_CIDR         CIDR passed to kubeadm init, must match Flannel (default 10.244.0.0/16)
#
# Conventions (fixed by platform-spawner):
#   - node-1 is the control plane, advertised on 172.30.100.101
#   - Workers are node-2 .. node-N
#   - Control-plane network is 172.30.100.0/24
#
# All container runtime / CNI components are pulled at their latest release
# from GitHub; the resolved versions are logged for traceability.

set -euo pipefail

: "${NODE_COUNT:?NODE_COUNT must be set}"
: "${KUBE_VERSION:?KUBE_VERSION must be set (e.g. v1.34.9)}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"

CONTROL_PLANE_NODE="${CONTROL_PLANE_NODE:-node-1}"
CONTROL_PLANE_ENDPOINT="${CONTROL_PLANE_ENDPOINT:-172.30.100.101}"

# Strip leading 'v' for the package repo URL (uses minor only, e.g. 1.34).
KUBE_MINOR="$(echo "${KUBE_VERSION#v}" | cut -d. -f1,2)"

SSH_OPTS=(-F ssh_config -o LogLevel=ERROR)

ssh_node() {
  local node="$1"; shift
  ssh "${SSH_OPTS[@]}" "$node" "$@"
}

ssh_sudo() {
  local node="$1"; shift
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$node" "sudo bash -s" <<EOF
set -euo pipefail
$*
EOF
}

scp_to() {
  local node="$1" src="$2" dst="$3"
  scp "${SSH_OPTS[@]}" "$src" "${node}:${dst}"
}

# --------------------------------------------------------------------------
# Resolve "latest" versions for runtime components.
# --------------------------------------------------------------------------
resolve_latest() {
  local repo="$1"
  curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" \
    | grep -oP '"tag_name":\s*"\K[^"]+'
}

echo "==> Resolving latest releases"
CONTAINERD_TAG="$(resolve_latest containerd/containerd)"       # e.g. v2.2.5
RUNC_TAG="$(resolve_latest opencontainers/runc)"                # e.g. v1.3.6
echo "    containerd:  ${CONTAINERD_TAG}"
echo "    runc:        ${RUNC_TAG}"
echo "    kubernetes:  ${KUBE_VERSION} (repo minor ${KUBE_MINOR})"

CONTAINERD_VER="${CONTAINERD_TAG#v}"
CONTAINERD_URL="https://github.com/containerd/containerd/releases/download/${CONTAINERD_TAG}/containerd-${CONTAINERD_VER}-linux-amd64.tar.gz"
CONTAINERD_SVC_URL="https://raw.githubusercontent.com/containerd/containerd/${CONTAINERD_TAG}/containerd.service"
RUNC_URL="https://github.com/opencontainers/runc/releases/download/${RUNC_TAG}/runc.amd64"
# Flannel's DaemonSet ships and installs the CNI reference binaries it needs
# (bridge, host-local, portmap, loopback, flannel) into /opt/cni/bin via an
# init container, so we don't pre-install the containernetworking/plugins
# bundle here.
FLANNEL_MANIFEST="https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml"

# --------------------------------------------------------------------------
# Build the node list.
# --------------------------------------------------------------------------
NODES=()
for i in $(seq 1 "$NODE_COUNT"); do
  NODES+=("node-$i")
done
echo "==> Target nodes: ${NODES[*]}"

# /etc/hosts block applied to every node so that 'node-i' resolves to its
# control-plane IP everywhere (kubeadm, kubelet logs, kubectl exec, etc.).
HOSTS_BLOCK="# >>> prepare-cluster.sh >>>
172.30.100.99 bastion"
for i in $(seq 1 "$NODE_COUNT"); do
  HOSTS_BLOCK+=$'\n'"172.30.100.$((100 + i)) node-$i"
done
HOSTS_BLOCK+=$'\n'"# <<< prepare-cluster.sh <<<"

# --------------------------------------------------------------------------
# Per-node bootstrap: kernel, swap, container runtime, kube binaries.
# --------------------------------------------------------------------------
bootstrap_node() {
  local node="$1"
  echo "==> [${node}] bootstrap"

  # Set the hostname to match the SSH alias and seed /etc/hosts so every
  # node can resolve its peers by name on the control-plane network.
  ssh_sudo "$node" "
    hostnamectl set-hostname '${node}'
    # Remove any previous block we wrote, then append the current one.
    sed -i '/# >>> prepare-cluster.sh >>>/,/# <<< prepare-cluster.sh <<</d' /etc/hosts
    cat >>/etc/hosts <<'EOH'
${HOSTS_BLOCK}
EOH
  "

  ssh_sudo "$node" "
    # Kernel modules required by kubeadm.
    cat >/etc/modules-load.d/k8s.conf <<'EOM'
overlay
br_netfilter
EOM
    modprobe overlay
    modprobe br_netfilter

    # Sysctls required by kubeadm.
    cat >/etc/sysctl.d/k8s.conf <<'EOM'
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOM
    sysctl --system >/dev/null

    # Disable swap (kubelet refuses to start otherwise on default config).
    swapoff -a
    sed -ri 's/^([^#].*\\sswap\\s)/#\\1/' /etc/fstab || true

    # SELinux: kubeadm docs recommend permissive for the lab.
    if command -v setenforce >/dev/null 2>&1; then
      setenforce 0 || true
      sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config || true
    fi

    # firewalld is enabled by default on Rocky; disable it for the CI cluster.
    if systemctl is-enabled --quiet firewalld 2>/dev/null; then
      systemctl disable --now firewalld
    fi
  "

  # containerd
  ssh_sudo "$node" "
    curl -fsSL '${CONTAINERD_URL}' | tar -C /usr/local -xz
    curl -fsSL '${CONTAINERD_SVC_URL}' -o /etc/systemd/system/containerd.service
    mkdir -p /etc/containerd
    /usr/local/bin/containerd config default >/etc/containerd/config.toml
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
    # Point the CRI image registry config_path at /etc/containerd/certs.d so
    # per-registry hosts.toml drop-ins can be added later without restarting.
    sed -i \"s|config_path = ''|config_path = '/etc/containerd/certs.d'|\" /etc/containerd/config.toml
    mkdir -p /etc/containerd/certs.d
    systemctl daemon-reload
    systemctl enable --now containerd
  "

  # runc
  ssh_sudo "$node" "
    curl -fsSL '${RUNC_URL}' -o /usr/local/sbin/runc
    chmod 755 /usr/local/sbin/runc
  "

  # Kubernetes packages from pkgs.k8s.io
  ssh_sudo "$node" "
    cat >/etc/yum.repos.d/kubernetes.repo <<EOM
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v${KUBE_MINOR}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v${KUBE_MINOR}/rpm/repodata/repomd.xml.key
exclude=kubelet kubeadm kubectl cri-tools kubernetes-cni
EOM

    KUBE_RPM_VER='${KUBE_VERSION#v}'
    dnf install -y --disableexcludes=kubernetes \
      kubelet-\${KUBE_RPM_VER} kubeadm-\${KUBE_RPM_VER} kubectl-\${KUBE_RPM_VER}
    systemctl enable --now kubelet
  "
}

for node in "${NODES[@]}"; do
  bootstrap_node "$node"
done

# --------------------------------------------------------------------------
# kubeadm init on the control plane.
# --------------------------------------------------------------------------
echo "==> Running kubeadm init on ${CONTROL_PLANE_NODE} (${CONTROL_PLANE_ENDPOINT})"
ssh_sudo "$CONTROL_PLANE_NODE" "
  kubeadm init \
    --kubernetes-version='${KUBE_VERSION}' \
    --pod-network-cidr='${POD_CIDR}' \
    --apiserver-advertise-address='${CONTROL_PLANE_ENDPOINT}' \
    --control-plane-endpoint='${CONTROL_PLANE_ENDPOINT}' \
    --node-name='${CONTROL_PLANE_NODE}'

  mkdir -p /root/.kube
  cp -f /etc/kubernetes/admin.conf /root/.kube/config

  # Make kubectl usable for the 'rocky' SSH user as well.
  install -d -o rocky -g rocky /home/rocky/.kube
  install -m 600 -o rocky -g rocky /etc/kubernetes/admin.conf /home/rocky/.kube/config
"

# --------------------------------------------------------------------------
# Join workers.
# --------------------------------------------------------------------------
echo "==> Generating join command"
JOIN_CMD="$(ssh_node "$CONTROL_PLANE_NODE" "sudo kubeadm token create --print-join-command")"
if [[ -z "${JOIN_CMD}" ]]; then
  echo "Failed to obtain kubeadm join command" >&2
  exit 1
fi

for i in $(seq 2 "$NODE_COUNT"); do
  worker="node-$i"
  echo "==> Joining ${worker}"
  ssh_sudo "$worker" "${JOIN_CMD} --node-name='${worker}'"
done

# --------------------------------------------------------------------------
# Open an sshuttle tunnel through the bastion so the runner can reach the
# control-plane (172.30.100.0/24) and workload-plane (172.30.200.0/24)
# networks directly, then fetch the kubeconfig from node-1.
# --------------------------------------------------------------------------
echo "==> Starting sshuttle through bastion"
sshuttle --ssh-cmd 'ssh -F ssh_config' --daemon \
  -r bastion 172.30.100.0/24 172.30.200.0/24

echo "==> Fetching kubeconfig from ${CONTROL_PLANE_NODE} to ./kubeconfig"
ssh_node "$CONTROL_PLANE_NODE" "sudo cat /etc/kubernetes/admin.conf" >kubeconfig
chmod 600 kubeconfig
# Sanity check: confirm the runner can reach the API server through sshuttle.
KUBECONFIG="$(pwd)/kubeconfig" kubectl version --request-timeout=10s >/dev/null

# --------------------------------------------------------------------------
# Deploy Flannel and wait for the cluster to settle.
# --------------------------------------------------------------------------
echo "==> Deploying Flannel"
ssh_sudo "$CONTROL_PLANE_NODE" "
  export KUBECONFIG=/etc/kubernetes/admin.conf
  kubectl apply -f '${FLANNEL_MANIFEST}'
"

echo "==> Waiting for nodes to become Ready"
ssh_sudo "$CONTROL_PLANE_NODE" "
  export KUBECONFIG=/etc/kubernetes/admin.conf
  kubectl wait --for=condition=Ready nodes --all --timeout=5m
  kubectl get nodes -o wide
"

# --------------------------------------------------------------------------
# Deploy cert-manager (latest stable release) and wait for it to be ready.
# --------------------------------------------------------------------------
CERT_MANAGER_TAG="$(resolve_latest cert-manager/cert-manager)"
echo "==> Deploying cert-manager ${CERT_MANAGER_TAG}"
ssh_sudo "$CONTROL_PLANE_NODE" "
  export KUBECONFIG=/etc/kubernetes/admin.conf
  kubectl apply -f 'https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_TAG}/cert-manager.yaml'
  kubectl -n cert-manager rollout status deploy/cert-manager --timeout=5m
  kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=5m
  kubectl -n cert-manager rollout status deploy/cert-manager-cainjector --timeout=5m
"

echo "==> Cluster ready"
