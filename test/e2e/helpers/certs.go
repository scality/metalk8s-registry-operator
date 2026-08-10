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

// certs.go: e2e CA + cert-manager Issuer provisioning plus short-lived
// client certificate issuance. The CA Secret and Issuer materialised here
// are the ones the Registry CR references via .spec.server /
// .spec.agent.authentication.mtls; NewClientCert produces the cert the
// upload helper presents to the RNA's mTLS API.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureCA guarantees that the RegistryNamespace, its CA Secret (name
// CASecretName, keys tls.crt/tls.key/ca.crt) and a cert-manager Issuer
// wrapping that secret all exist. It is idempotent: calling it twice reuses
// whatever is already there. The registry namespace is intentionally used
// as the CA namespace because the operator's validating webhook resolves
// .spec.{server,agent}.certificateIssuerRef against that namespace.
//
// Returned bytes are the PEM-encoded CA certificate and private key,
// suitable for signing client certificates (see NewClientCert).
func EnsureCA(ctx context.Context, c client.Client) (caCertPEM, caKeyPEM []byte, err error) {
	if err := ensureNamespace(ctx, c, RegistryNamespace); err != nil {
		return nil, nil, err
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: RegistryNamespace, Name: CASecretName}
	if err := c.Get(ctx, key, secret); err == nil {
		if certPEM, keyPEM, ok := extractCA(secret); ok {
			if err := ensureIssuer(ctx, c); err != nil {
				return nil, nil, err
			}
			return certPEM, keyPEM, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, nil, err
	}

	certPEM, keyPEM, err := generateCA()
	if err != nil {
		return nil, nil, err
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CASecretName,
			Namespace: RegistryNamespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
			// ca.crt duplicates tls.crt so the same secret can be referenced
			// as a plain CA secret by .spec.agent.authentication.mtls.
			"ca.crt": certPEM,
		},
	}
	if err := c.Create(ctx, newSecret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, nil, err
		}
		// Someone raced us; read the current secret and honour its bytes.
		existing := &corev1.Secret{}
		if err := c.Get(ctx, key, existing); err != nil {
			return nil, nil, err
		}
		if p, k, ok := extractCA(existing); ok {
			certPEM, keyPEM = p, k
		} else {
			return nil, nil, fmt.Errorf(
				"existing CA Secret %s/%s has invalid or missing cert/key data",
				RegistryNamespace, CASecretName,
			)
		}
	}
	if err := ensureIssuer(ctx, c); err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// NewClientCert issues a fresh short-lived client cert + key PEM-encoded,
// signed by the given CA. commonName is used both as the CN and as the first
// DNS SAN, which is what the RNA upload API server verifies against.
func NewClientCert(caCertPEM, caKeyPEM []byte, commonName string) (certPEM, keyPEM []byte, err error) {
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		return nil, nil, fmt.Errorf("cannot decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		return nil, nil, fmt.Errorf("cannot decode CA key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{commonName},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &privKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})
	return certPEM, keyPEM, nil
}

// DeleteCAArtifacts removes the CA Issuer and Secret. The registry
// namespace is intentionally left alone: callers own that lifecycle.
func DeleteCAArtifacts(ctx context.Context, c client.Client) error {
	issuer := &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{Name: IssuerName, Namespace: RegistryNamespace},
	}
	if err := c.Delete(ctx, issuer); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: CASecretName, Namespace: RegistryNamespace},
	}
	if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func generateCA() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "metalk8s-registry-operator-e2e-ca"},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return certPEM, keyPEM, nil
}

func extractCA(s *corev1.Secret) (cert, key []byte, ok bool) {
	cert = s.Data[corev1.TLSCertKey]
	if len(cert) == 0 {
		cert = s.Data["ca.crt"]
	}
	key = s.Data[corev1.TLSPrivateKeyKey]
	return cert, key, len(cert) > 0 && len(key) > 0
}

func ensureNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := c.Create(ctx, ns)
	if err == nil || apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func ensureIssuer(ctx context.Context, c client.Client) error {
	issuer := &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IssuerName,
			Namespace: RegistryNamespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{SecretName: CASecretName},
			},
		},
	}
	err := c.Create(ctx, issuer)
	if err == nil || apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
