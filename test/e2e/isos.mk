# Synthetic ISO fixtures for the e2e suite. Each ISO is a raw ISO 9660 image
# whose root is a tree of `<image>/{oci-layout, index.json, blobs/sha256/*}`
# OCI Image Layout directories as understood by scality/static-oci-registry
# >= v0.1.0-beta.2 (deployed under the name metalk8s-registry-server). When
# mounted at <solutions>/<sa.name>/<sa.version>/ its contents can be pulled
# by containerd through the operator's containerd mirror. The tree paths
# *are* the pullable image references (host + repository).
#
# Each layout is materialised by test/e2e/build-fixture.sh, which builds a
# minimal `FROM scratch` image from test/e2e/testimage/Dockerfile using
# `docker build` and converts the result to an OCI Image Layout with
# `skopeo copy docker-daemon:… oci:<dir>:<tag>`. This mirrors the pattern
# used by scality/static-oci-registry's own integration tests and avoids
# maintaining a bespoke OCI marshaller in this repo.
#
# This fragment is intended to be included from the top-level Makefile; all
# paths are therefore relative to the repository root.

##@ E2E fixtures

# Shared configuration with test/e2e/helpers/fixtures.go — defines
# E2E_IMAGE_HOST, BULK_SMALL_INDICES, BULK_BIG_INDICES so the ISO build and
# the Go test suite stay in lockstep. Exported so `go test`, invoked via
# `make test-e2e`, sees them through os.Getenv.
include test/e2e/fixtures.env
export E2E_IMAGE_HOST BULK_SMALL_INDICES BULK_BIG_INDICES

DIST_ISOS ?= dist/test-isos
# Payload size (in whole MiB) for the images inside the "small" ISOs. Keep
# it small to make dev loops fast.
SMALL_ISO_SIZE_MB ?= 1
# Payload size (in whole MiB) for the image inside the "big" ISO.
# Overridable at the CLI: make test-e2e-isos BIG_ISO_SIZE_MB=200
BIG_ISO_SIZE_MB ?= 100

# Bulk fixtures for the "10 small + 2 big archives" step. Each index yields
# an ISO named `bulk-<kind>-<index>.iso` containing exactly one image at
# `$(E2E_IMAGE_HOST)/bulk-<kind>-<index>:v1`.
BULK_SMALL_ISOS := $(patsubst %,$(DIST_ISOS)/bulk-small-%.iso,$(BULK_SMALL_INDICES))
BULK_BIG_ISOS := $(patsubst %,$(DIST_ISOS)/bulk-big-%.iso,$(BULK_BIG_INDICES))

FIXTURE_SRC := test/e2e/build-fixture.sh test/e2e/testimage/Dockerfile

$(DIST_ISOS)/small-single.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/small-single
	./test/e2e/build-fixture.sh $(SMALL_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/small-single/$(E2E_IMAGE_HOST)/small-single v1
	genisoimage -quiet -o $@ -V SMALL_SINGLE -r -J $(DIST_ISOS)/build/small-single
	sha256sum $@ | awk '{print $$1}' > $@.sha256

$(DIST_ISOS)/small-multi.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/small-multi
	./test/e2e/build-fixture.sh $(SMALL_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/small-multi/$(E2E_IMAGE_HOST)/small-multi/app-a v1
	./test/e2e/build-fixture.sh $(SMALL_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/small-multi/$(E2E_IMAGE_HOST)/small-multi/app-b v1
	./test/e2e/build-fixture.sh $(SMALL_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/small-multi/$(E2E_IMAGE_HOST)/small-multi/app-c v1
	genisoimage -quiet -o $@ -V SMALL_MULTI -r -J $(DIST_ISOS)/build/small-multi
	sha256sum $@ | awk '{print $$1}' > $@.sha256

$(DIST_ISOS)/big.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/big
	./test/e2e/build-fixture.sh $(BIG_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/big/$(E2E_IMAGE_HOST)/big v1
	genisoimage -quiet -o $@ -V BIG -r -J $(DIST_ISOS)/build/big
	sha256sum $@ | awk '{print $$1}' > $@.sha256

$(DIST_ISOS)/big-extension.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/big-extension
	./test/e2e/build-fixture.sh $(BIG_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/big-extension/$(E2E_IMAGE_HOST)/big-extension v1
	genisoimage -quiet -o $@ -V BIG_EXTENSION -r -J $(DIST_ISOS)/build/big-extension
	sha256sum $@ | awk '{print $$1}' > $@.sha256

# Bulk small ISOs — pattern rule keyed on the two-digit index.
$(DIST_ISOS)/bulk-small-%.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/bulk-small-$*
	./test/e2e/build-fixture.sh $(SMALL_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/bulk-small-$*/$(E2E_IMAGE_HOST)/bulk-small-$* v1
	genisoimage -quiet -o $@ -V BULK_SMALL_$* -r -J $(DIST_ISOS)/build/bulk-small-$*
	sha256sum $@ | awk '{print $$1}' > $@.sha256

# Bulk big ISOs — pattern rule keyed on the two-digit index.
$(DIST_ISOS)/bulk-big-%.iso: $(FIXTURE_SRC)
	@mkdir -p $(DIST_ISOS)
	@rm -rf $(DIST_ISOS)/build/bulk-big-$*
	./test/e2e/build-fixture.sh $(BIG_ISO_SIZE_MB) \
		$(DIST_ISOS)/build/bulk-big-$*/$(E2E_IMAGE_HOST)/bulk-big-$* v1
	genisoimage -quiet -o $@ -V BULK_BIG_$* -r -J $(DIST_ISOS)/build/bulk-big-$*
	sha256sum $@ | awk '{print $$1}' > $@.sha256

.PHONY: test-e2e-isos
test-e2e-isos: $(DIST_ISOS)/small-single.iso $(DIST_ISOS)/small-multi.iso $(DIST_ISOS)/big.iso $(DIST_ISOS)/big-extension.iso $(BULK_SMALL_ISOS) $(BULK_BIG_ISOS) ## Build the ISO fixtures consumed by the e2e suite.
