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

// archive.go: SolutionArchive CR creation with automatic ISO checksumming,
// status summarisation, and NodeSolutionArchive listing/cleanup helpers.
// Wraps the operator's SolutionArchive API and the node-agent's
// NodeSolutionArchive CRD so specs don't need to import the latter.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// ArchiveSpec captures the inputs needed to create a SolutionArchive and
// upload its backing ISO. `Name` and `Version` end up as the on-disk
// namespace prefix, so `Name` must contain a `.` if you want images inside
// the archive to be pullable through the containerd _default mirror (that
// mirror is only invoked when containerd treats the image reference host as
// a registry, i.e. when the first path segment contains a dot or a colon).
type ArchiveSpec struct {
	// Name is the SolutionArchive .spec.name. Suggested convention:
	// "<something>.io" so containerd recognises it as a host.
	Name string
	// Version is the SolutionArchive .spec.version.
	Version string
	// ISOPath is the local filesystem path to the .iso to upload.
	ISOPath string
}

// ChecksumSHA256 returns the hex-encoded SHA-256 of the ISO file.
func (a ArchiveSpec) ChecksumSHA256() (string, error) {
	f, err := os.Open(a.ISOPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// EnsureArchive creates a SolutionArchive CR with the checksum computed from
// ISOPath. Returns the resource so tests can call Get()/status assertions on
// it via k8sClient.
func EnsureArchive(ctx context.Context, c client.Client, spec ArchiveSpec) (*metalk8sv1alpha1.SolutionArchive, error) {
	sum, err := spec.ChecksumSHA256()
	if err != nil {
		return nil, fmt.Errorf("checksum %s: %w", spec.ISOPath, err)
	}

	sa := &metalk8sv1alpha1.SolutionArchive{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", spec.Name, spec.Version),
		},
		Spec: rnav1alpha1.SolutionArchiveSpec{
			Name:    spec.Name,
			Version: spec.Version,
			Validation: &rnav1alpha1.SolutionArchiveValidation{
				Checksum: rnav1alpha1.SolutionArchiveChecksum{
					Type:  "sha256",
					Value: sum,
				},
			},
		},
	}

	if err := c.Create(ctx, sa); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create SolutionArchive %s: %w", sa.Name, err)
		}
		if err := c.Get(ctx, client.ObjectKey{Name: sa.Name}, sa); err != nil {
			return nil, err
		}
	}
	return sa, nil
}

// DeleteArchive removes a SolutionArchive by (name, version).
func DeleteArchive(ctx context.Context, c client.Client, name, version string) error {
	sa := &metalk8sv1alpha1.SolutionArchive{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", name, version),
		},
	}
	if err := c.Delete(ctx, sa); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// ArchiveStatusSummary flattens SolutionArchive.status for concise assertions.
type ArchiveStatusSummary struct {
	Served         bool
	Replicated     bool
	ServedReplicas int
	TargetReplicas int
	NodeArchives   []string
}

// SummarizeArchive turns SA status pointers into a plain-value view.
func SummarizeArchive(sa *metalk8sv1alpha1.SolutionArchive) ArchiveStatusSummary {
	return ArchiveStatusSummary{
		Served:         ptr.Deref(sa.Status.Served, false),
		Replicated:     ptr.Deref(sa.Status.Replicated, false),
		ServedReplicas: ptr.Deref(sa.Status.ServedReplicas, 0),
		TargetReplicas: ptr.Deref(sa.Status.TargetReplicas, 0),
		NodeArchives:   append([]string(nil), sa.Status.NodeSolutionArchives...),
	}
}

// GetNodeArchives fetches every NodeSolutionArchive belonging to a
// SolutionArchive. Wrapping the node-agent CRD list keeps the tests from
// needing to import the node-agent's typed API.
func GetNodeArchives(
	ctx context.Context, c client.Client, saName, version string,
) (*rnav1alpha1.NodeSolutionArchiveList, error) {
	list := &rnav1alpha1.NodeSolutionArchiveList{}
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	filtered := list.Items[:0]
	for _, nsa := range list.Items {
		if nsa.Spec.Name == saName && nsa.Spec.Version == version {
			filtered = append(filtered, nsa)
		}
	}
	list.Items = filtered
	return list, nil
}

// ForceCleanupNodeArchives strips the metalk8s finalizer from every
// NodeSolutionArchive matching the given name prefix. It is a last-resort
// escape hatch for the e2e cleanup path: NSAs are normally deleted when
// their parent SolutionArchive goes away, but their finalizer requires the
// owning RNA pod to run the on-node unmount. If that RNA is no longer on
// the node — for example because the taint test evicted it, or because the
// node was unlabelled before the NSA finished terminating — the NSA can
// stay stuck in Terminating forever, which in turn blocks garbage
// collection of the parent SolutionArchive and of the Registry itself.
// Removing the finalizer lets the operator continue and the next Describe
// start from a clean slate.
func ForceCleanupNodeArchives(ctx context.Context, c client.Client, namePrefix string) (int, error) {
	list := &rnav1alpha1.NodeSolutionArchiveList{}
	if err := c.List(ctx, list); err != nil {
		return 0, err
	}
	stripped := 0
	for i := range list.Items {
		nsa := &list.Items[i]
		if namePrefix != "" && !strings.HasPrefix(nsa.Name, namePrefix) {
			continue
		}
		if len(nsa.Finalizers) == 0 {
			continue
		}
		patch := client.MergeFrom(nsa.DeepCopy())
		nsa.Finalizers = nil
		if err := c.Patch(ctx, nsa, patch); err != nil && !apierrors.IsNotFound(err) {
			return stripped, fmt.Errorf("force-strip finalizer on %s: %w", nsa.Name, err)
		}
		stripped++
	}
	return stripped, nil
}
