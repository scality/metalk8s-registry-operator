# Minimal fixture image used by the e2e suite. The build context must
# contain a `payload.bin` file whose size determines the ISO footprint;
# the resulting image has a single layer with `/payload.bin` and no
# executable content. containerd can pull it through the operator's
# mirror; the container itself cannot run — `/payload.bin` is random data,
# not an ELF — but that is fine for the tests, which only assert on pull
# success (kubelet reports the container `ImageID` as soon as the spec is
# generated). A `CMD` is nevertheless required so kubelet does not reject
# the container spec with "no command specified" before observing the
# `ImageID`; without it we would get stuck in `CreateContainerError`.
FROM scratch
ARG VERSION=v1
LABEL org.opencontainers.image.title="metalk8s-registry-operator e2e fixture"
LABEL org.opencontainers.image.version="${VERSION}"
COPY payload.bin /payload.bin
CMD ["/payload.bin"]

