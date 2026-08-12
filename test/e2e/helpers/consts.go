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

// Package helpers contains support code shared by the e2e specs.
//
// Every helper is written to be safe to call multiple times against a live
// cluster: creations are idempotent (CreateOrUpdate patterns), and cleanup
// helpers tolerate absent objects. Nothing in this package starts a
// controller manager in-process; the tests always talk to the real operator
// deployed on the target cluster.
package helpers

const (
	// RegistryRoleLabel is the label key that the Registry CR selects on to
	// place its StatefulSets. It is applied to and removed from nodes by
	// LabelAsRegistry and UnlabelAsRegistry.
	RegistryRoleLabel = "node-role.kubernetes.io/registry"

	// RegistryNamespace is the target namespace of the registry deployment.
	// This value has to match the .spec.namespace of the Registry CR passed
	// to CreateRegistry; the CA secret and the cert-manager Issuer both
	// live here so the operator's validating webhook can find them (it
	// resolves the Issuer against `registry.GetRegistryNamespace()`).
	RegistryNamespace = "metalk8s-registry"

	// CASecretName is the name of the CA Secret used both for
	//   .spec.server.certificateIssuerRef (via a matching Issuer wrapping
	//   this secret) and
	//   .spec.agent.authentication.mtls.caSecretRef.
	CASecretName = "e2e-ca"

	// IssuerName is the cert-manager Issuer (in RegistryNamespace) that
	// wraps the CA secret and is referenced by the Registry
	// .spec.{server,agent}.certificateIssuerRef fields.
	IssuerName = "e2e-ca-issuer"

	// RegistryPullSecretName is the name of the Docker-config-JSON Secret
	// in RegistryNamespace used to pull the RNA + Server images from
	// private registries (typically ghcr.io/scality/...). The deploy CI
	// job creates this Secret alongside the operator; NewRegistrySpec
	// wires it into .spec.{agent,server}.image.pullSecrets so the
	// operator's rendered StatefulSets pick it up.
	RegistryPullSecretName = "registry-pull-secret"

	// RegistryServerServiceName is the ClusterIP Service in the registry
	// namespace that fronts the static-oci-registry pods.
	RegistryServerServiceName = "metalk8s-registry-server"

	// RegistryServerPort is the port the server listens on.
	RegistryServerPort = 5000
)
