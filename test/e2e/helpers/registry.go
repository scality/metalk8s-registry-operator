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

// registry.go: creation, status polling, and cleanup helpers for the
// operator's Registry CR, plus per-node TLS Secret wiping used between
// consecutive Describe blocks to force cert-manager to re-issue certs
// against a fresh Service ClusterIP.

import (
	"context"
	"fmt"
	"strings"

	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// DefaultRegistryName is the standard name for the Registry CR used by all
// specs.
const DefaultRegistryName = "e2e-registry"

// NewRegistrySpec returns a RegistrySpec pointing at the CA/Issuer provisioned
// by EnsureCA and selecting nodes carrying the RegistryRoleLabel. All fields
// referenced by the operator's status computation are set so no field
// defaulting from the mutating webhook is required.
func NewRegistrySpec() metalk8sv1alpha1.RegistrySpec {
	issuerRef := cmmetav1.ObjectReference{
		Name:  IssuerName,
		Kind:  "Issuer",
		Group: "cert-manager.io",
	}
	pullSecrets := []corev1.LocalObjectReference{
		{Name: RegistryPullSecretName},
	}
	return metalk8sv1alpha1.RegistrySpec{
		LogLevel:  ptr.To("info"),
		Namespace: ptr.To(RegistryNamespace),
		NodeSelector: map[string]string{
			RegistryRoleLabel: "",
		},
		Server: metalk8sv1alpha1.RegistryServerSpec{
			CertificateIssuerRef: issuerRef,
			Image: &metalk8sv1alpha1.ImageSpec{
				PullSecrets: pullSecrets,
			},
		},
		Agent: metalk8sv1alpha1.RegistryNodeAgentSpec{
			CertificateIssuerRef: issuerRef,
			Image: &metalk8sv1alpha1.ImageSpec{
				PullSecrets: pullSecrets,
			},
			Authentication: metalk8sv1alpha1.AuthenticationSpec{
				MTLS: metalk8sv1alpha1.MTLSAuthenticationSpec{
					CASecretRef: corev1.SecretReference{
						Name:      CASecretName,
						Namespace: RegistryNamespace,
					},
				},
			},
		},
	}
}

// EnsureRegistry creates (or reuses) a Registry CR named DefaultRegistryName
// with NewRegistrySpec().
func EnsureRegistry(ctx context.Context, c client.Client) (*metalk8sv1alpha1.Registry, error) {
	reg := &metalk8sv1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultRegistryName},
		Spec:       NewRegistrySpec(),
	}
	if err := c.Create(ctx, reg); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create Registry: %w", err)
		}
		if err := c.Get(ctx, client.ObjectKey{Name: DefaultRegistryName}, reg); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// GetRegistry fetches the standard Registry CR.
func GetRegistry(ctx context.Context, c client.Client) (*metalk8sv1alpha1.Registry, error) {
	reg := &metalk8sv1alpha1.Registry{}
	if err := c.Get(ctx, client.ObjectKey{Name: DefaultRegistryName}, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

// DeleteRegistry removes the Registry CR (idempotent).
func DeleteRegistry(ctx context.Context, c client.Client) error {
	reg := &metalk8sv1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultRegistryName},
	}
	if err := c.Delete(ctx, reg); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// ForceRemoveRegistryFinalizers strips every finalizer from the Registry CR
// via a JSON-merge patch (bypassing the operator's Update flow). It is an
// emergency escape hatch used by the cleanup path when the Registry has
// been Deleted but the operator's finalizer has not yet completed within
// the timeout — for example because the operator pod is down, or some
// resource it owns is itself stuck terminating. Without this fallback a
// hung Registry would block every subsequent Describe from creating a
// fresh one with the same name.
//
// If the Registry does not exist, this is a no-op.
func ForceRemoveRegistryFinalizers(ctx context.Context, c client.Client) error {
	reg, err := GetRegistry(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(reg.Finalizers) == 0 {
		return nil
	}
	patch := client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`))
	if err := c.Patch(ctx, reg, patch); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("strip Registry finalizers: %w", err)
	}
	return nil
}

// SetRegistryMonitoring replaces .spec.monitoring of the standard Registry CR.
func SetRegistryMonitoring(ctx context.Context, c client.Client, monitoring *metalk8sv1alpha1.MonitoringSpec) error {
	reg, err := GetRegistry(ctx, c)
	if err != nil {
		return err
	}
	patch := client.MergeFrom(reg.DeepCopy())
	reg.Spec.Monitoring = monitoring
	if err := c.Patch(ctx, reg, patch); err != nil {
		return fmt.Errorf("patch Registry monitoring: %w", err)
	}
	return nil
}

// DeletePerNodeTLSSecrets removes the per-node TLS Secrets that cert-manager
// materialises for the RS and RNA StatefulSets. These Secrets are created by
// cert-manager on behalf of Certificate resources owned by the Registry CR,
// but the Secret objects themselves are NOT owned by anything — so deleting
// the Registry does not garbage-collect them. If the next Registry lands in
// the same namespace with a *different* Service ClusterIP, the RS pod may
// start reading the still-cached previous cert (containing the old
// ClusterIP) before cert-manager finishes re-issuing, leading to
// intermittent TLS SAN mismatch failures on containerd pulls.
//
// Wiping these Secrets on cleanup guarantees the next Registry always starts
// with freshly-issued certificates matching the new ClusterIP.
func DeletePerNodeTLSSecrets(ctx context.Context, c client.Client) error {
	secrets := &corev1.SecretList{}
	if err := c.List(ctx, secrets, client.InNamespace(RegistryNamespace)); err != nil {
		return fmt.Errorf("listing secrets in %s: %w", RegistryNamespace, err)
	}
	for i := range secrets.Items {
		s := &secrets.Items[i]
		if !isPerNodeTLSSecret(s.Name) {
			continue
		}
		if err := c.Delete(ctx, s); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("deleting Secret %s/%s: %w", s.Namespace, s.Name, err)
		}
	}
	return nil
}

// isPerNodeTLSSecret returns true iff the Secret name matches the per-node
// TLS Secret naming scheme used by the operator (see
// internal/controller/certificate_reconciler.go). The CA Secret and the
// operator's own webhook/metrics certificates are left untouched — they are
// not per-node and their SANs are static.
func isPerNodeTLSSecret(name string) bool {
	prefixes := []string{
		"rs-external-server-",
		"rna-external-server-",
		"rna-internal-server-",
		"rna-internal-client-",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return name == "rna-external-client"
}

// RegistryStatusSummary is a compact view of Registry.status fields for use
// in Gomega assertions. Pointers are unwrapped to plain values (nil pointers
// become the zero value); the caller can compare fields directly.
type RegistryStatusSummary struct {
	Replicas            int
	ReadyServerReplicas int
	ReadyAgentReplicas  int
	Available           bool
	Ready               bool
	MirrorSyncReady     bool
	SelectedNodes       []string
	ClusterIP           string
	NodeIPs             []string
}

// Summarize collapses reg.Status into a RegistryStatusSummary.
func Summarize(reg *metalk8sv1alpha1.Registry) RegistryStatusSummary {
	return RegistryStatusSummary{
		Replicas:            ptr.Deref(reg.Status.Replicas, 0),
		ReadyServerReplicas: ptr.Deref(reg.Status.ReadyServerReplicas, 0),
		ReadyAgentReplicas:  ptr.Deref(reg.Status.ReadyAgentReplicas, 0),
		Available:           ptr.Deref(reg.Status.Available, false),
		Ready:               ptr.Deref(reg.Status.Ready, false),
		MirrorSyncReady:     ptr.Deref(reg.Status.MirrorSyncReady, false),
		SelectedNodes:       append([]string(nil), reg.Status.SelectedNodes...),
		ClusterIP:           reg.Status.ClusterIP,
		NodeIPs:             append([]string(nil), reg.Status.NodeIPs...),
	}
}
