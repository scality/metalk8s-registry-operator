#!/usr/bin/env bash
# build-fixture.sh — build one OCI Image Layout directory for the e2e suite.
#
# Usage: build-fixture.sh <size-mib> <out-dir> <tag>
#
# Generates a `<size-mib>` MiB payload of random bytes (uncompressible, so
# the layer size on disk matches the requested size), builds a `FROM scratch`
# image containing it, and exports the image to `<out-dir>` as an OCI Image
# Layout with the given `<tag>` (materialising `oci-layout`, `index.json`
# and `blobs/sha256/*`).
#
# The `<out-dir>` becomes one leaf of an ISO tree consumed by
# scality/static-oci-registry (deployed as metalk8s-registry-server): the
# tree paths *are* the pullable image references (host + repository), so
# callers pass a nested directory like
# `dist/test-isos/build/small-single/e2e.metalk8s.scality.com/small-single`.
#
# Requires `docker` (build daemon) and `skopeo` on PATH.
set -euo pipefail

if [[ $# -ne 3 ]]; then
	echo "usage: $0 <size-mib> <out-dir> <tag>" >&2
	exit 2
fi

size_mib=$1
out_dir=$2
tag=$3

if ! [[ "${size_mib}" =~ ^[0-9]+$ ]]; then
	echo "$0: size-mib must be a positive integer (got '${size_mib}')" >&2
	exit 2
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
dockerfile=${script_dir}/testimage/Dockerfile

if [[ ! -f "${dockerfile}" ]]; then
	echo "$0: cannot find Dockerfile at ${dockerfile}" >&2
	exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "${tmp}"' EXIT

dd if=/dev/urandom of="${tmp}/payload.bin" bs=1M count="${size_mib}" status=none
cp "${dockerfile}" "${tmp}/Dockerfile"

# Unique per-invocation tag so parallel Make jobs don't collide.
local_tag="localhost/metalk8s-registry-operator/e2e-fixture-$$-${RANDOM}:${tag}"

docker build --quiet --build-arg "VERSION=${tag}" \
	-t "${local_tag}" "${tmp}" >/dev/null

# Ensure the destination layout dir is empty; skopeo copy would otherwise
# accumulate tags across invocations and the ISO would carry stale blobs.
rm -rf "${out_dir}"
mkdir -p "${out_dir}"

skopeo copy --quiet --insecure-policy \
	"docker-daemon:${local_tag}" \
	"oci:${out_dir}:${tag}"

docker rmi -f "${local_tag}" >/dev/null
