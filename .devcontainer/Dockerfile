FROM mcr.microsoft.com/devcontainers/go:1.27-trixie@sha256:06ae646a1f1ec51551e5abd15954f9279939953eeb74482052efcaaad526ab0d

USER vscode

# renovate: datasource=github-releases depName=kubernetes-sigs/kubebuilder extractVersion=^v(?<version>.*)$
ARG KUBEBUILDER_VERSION=4.6.0

RUN curl -sSfLO https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_linux_amd64 && \
    chmod +x kubebuilder_linux_amd64 && \
    sudo mv kubebuilder_linux_amd64 /usr/local/bin/kubebuilder

# renovate: datasource=github-releases depName=golangci/golangci-lint extractVersion=^v(?<version>.*)$
ARG GOLANGCI_LINT_VERSION=2.15.0

RUN curl -sSfL  https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sudo sh -s -- v${GOLANGCI_LINT_VERSION}

# renovate: datasource=github-releases depName=kubernetes/kubernetes extractVersion=^v(?<version>.*)$
ARG KUBECTL_VERSION=1.37.1

RUN curl -sSfLO https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl && \
    chmod +x kubectl && \
    sudo mv kubectl /usr/local/bin/kubectl
