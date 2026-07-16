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

package helpers

// fixtures.go: declarative catalogue of the prebuilt ISO fixtures the e2e
// suite pulls images from. The Fixture struct carries just the identifying
// bits (solution name/version, filename slug, optional multi-image repos);
// derived paths and pullable image references are produced by methods so
// no bespoke code path per fixture is needed. Shared configuration
// (registry hostname, bulk indices) is loaded once from the process
// environment; test/e2e/isos.mk exports the values so `make test-e2e`
// always populates them, and CI can also set them directly.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Fixture describes one prebuilt e2e ISO together with everything a spec
// needs to upload it (ISOPath, ArchiveSpec) and to verify that its contents
// are pullable through the containerd mirror (PullRefs).
type Fixture struct {
	// SolutionName is the SolutionArchive .spec.name for this fixture.
	SolutionName string
	// SolutionVersion is the SolutionArchive .spec.version.
	SolutionVersion string
	// Slug is both the ISO filename base (`<slug>.iso`) and the top-level
	// repository name inside the ISO's OCI Image Layout tree, so
	// containerd pulls `<E2EImageHost>/<slug>[/subrepo]:v1`.
	Slug string
	// ExtraRepos, when non-empty, expands PullRefs into
	// `<E2EImageHost>/<slug>/<repo>:v1` for each entry. Nil/empty means
	// the fixture contains exactly one image at `<E2EImageHost>/<slug>:v1`.
	ExtraRepos []string
}

// ISOPath returns the on-disk path to the prebuilt ISO for this fixture.
func (f Fixture) ISOPath() string {
	return filepath.Join(FixtureRoot(), f.Slug+".iso")
}

// PullRefs returns every image reference pullable from this fixture,
// fully qualified with the registry host and tag.
func (f Fixture) PullRefs() []string {
	if len(f.ExtraRepos) == 0 {
		return []string{fmt.Sprintf("%s/%s:v1", E2EImageHost, f.Slug)}
	}
	refs := make([]string, len(f.ExtraRepos))
	for i, repo := range f.ExtraRepos {
		refs[i] = fmt.Sprintf("%s/%s/%s:v1", E2EImageHost, f.Slug, repo)
	}
	return refs
}

// ArchiveSpec is a convenience adapter to helpers.ArchiveSpec for callers
// that already have a Fixture value.
func (f Fixture) ArchiveSpec() ArchiveSpec {
	return ArchiveSpec{
		Name:    f.SolutionName,
		Version: f.SolutionVersion,
		ISOPath: f.ISOPath(),
	}
}

// FixtureRoot returns the on-disk directory where Makefile targets deposit
// generated ISOs. Precedence:
//
//  1. `E2E_ISO_DIR` env var (absolute or relative to the test binary's CWD).
//  2. The repository's `dist/test-isos` directory, located by walking three
//     levels up from this source file (test/e2e/helpers/fixtures.go).
//
// The tests only run in two contexts — from a developer machine via the
// Makefile, or from CI — and in both cases this source file's location is
// stable relative to the repository root, so runtime.Caller is enough.
func FixtureRoot() string {
	if v := os.Getenv("E2E_ISO_DIR"); v != "" {
		return v
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "dist/test-isos"
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "dist", "test-isos")
}

// E2EImageHost is the registry hostname baked into every image reference
// inside the generated ISO fixtures. Set from the E2E_IMAGE_HOST env var
// (exported by test/e2e/isos.mk from test/e2e/fixtures.env).
var E2EImageHost = os.Getenv("E2E_IMAGE_HOST")

// BulkSmallIndices is the exact list of two-digit indices consumed by the
// Makefile's bulk-small pattern rule. Loaded from the BULK_SMALL_INDICES
// env var.
var BulkSmallIndices = strings.Fields(os.Getenv("BULK_SMALL_INDICES"))

// BulkBigIndices matches the Makefile's bulk-big pattern rule. Loaded
// from the BULK_BIG_INDICES env var.
var BulkBigIndices = strings.Fields(os.Getenv("BULK_BIG_INDICES"))

// Prebuilt named fixtures. Bulk fixtures below are parametrised by index so
// they stay as functions.
var (
	SmallSingle = Fixture{
		SolutionName:    "e2e-small-single",
		SolutionVersion: "v1.0.0",
		Slug:            "small-single",
	}
	SmallMulti = Fixture{
		SolutionName:    "e2e-small-multi",
		SolutionVersion: "v1.0.0",
		Slug:            "small-multi",
		ExtraRepos:      []string{"app-a", "app-b", "app-c"},
	}
	Big = Fixture{
		SolutionName:    "e2e-big",
		SolutionVersion: "v1.0.0",
		Slug:            "big",
	}
	BigExtension = Fixture{
		SolutionName:    "e2e-big-extension",
		SolutionVersion: "v1.0.0",
		Slug:            "big-extension",
	}
)

// BulkSmallFixture returns the fixture describing bulk-small-<index>.iso.
func BulkSmallFixture(index string) Fixture {
	return Fixture{
		SolutionName:    "e2e-bulk-small-" + index,
		SolutionVersion: "v1.0.0",
		Slug:            "bulk-small-" + index,
	}
}

// BulkBigFixture returns the fixture describing bulk-big-<index>.iso.
func BulkBigFixture(index string) Fixture {
	return Fixture{
		SolutionName:    "e2e-bulk-big-" + index,
		SolutionVersion: "v1.0.0",
		Slug:            "bulk-big-" + index,
	}
}

// BulkFixtures returns every bulk fixture (10 smalls followed by 2 bigs)
// in a stable order.
func BulkFixtures() []Fixture {
	out := make([]Fixture, 0, len(BulkSmallIndices)+len(BulkBigIndices))
	for _, i := range BulkSmallIndices {
		out = append(out, BulkSmallFixture(i))
	}
	for _, i := range BulkBigIndices {
		out = append(out, BulkBigFixture(i))
	}
	return out
}

// init validates that the env-var-backed globals were populated. If any is
// empty, the developer has invoked `go test ./test/e2e/...` directly
// instead of going through `make test-e2e` — surface a clear error so they
// know how to fix it.
func init() {
	if E2EImageHost == "" || len(BulkSmallIndices) == 0 || len(BulkBigIndices) == 0 {
		panic("fixture env vars unset — run the e2e suite via `make test-e2e` " +
			"(which exports E2E_IMAGE_HOST, BULK_SMALL_INDICES, BULK_BIG_INDICES " +
			"from test/e2e/fixtures.env) or export them manually before `go test`")
	}
}
