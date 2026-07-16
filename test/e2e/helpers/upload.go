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

// upload.go: chunked, mTLS-authenticated ISO upload against the
// registry-node-agent's HTTP API. Resolves the target node's InternalIP
// and hits its `:5001` hostPort directly — the same endpoint a production
// MetalK8s client would use. This assumes the test runner has network
// reachability to the cluster's control-plane subnet (via sshuttle in dev
// and via the same tunnel opened by .github/scripts/prepare-cluster.sh in
// CI).

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// rnaUploadPort is the port the RNA container exposes as a hostPort for
// its external (upload) HTTP API. See
// metalk8s-registry-node-agent/config/manager/manager.yaml.
const rnaUploadPort = 5001

// uploadChunkSize is the byte length of each PUT request the client makes
// against the RNA upload API. The receiver accepts any contiguous
// Content-Range, so this is a plain trade-off between per-chunk overhead
// (HTTP request/response for each) and memory pressure (bigger chunks =
// bigger buffers). 1 MiB is a safe sweet spot; the node-agent's own e2e
// uses 45 KiB but that is unnecessarily conservative for a big-archive path.
const uploadChunkSize = 1 << 20 // 1 MiB

// UploadArchiveISO uploads the ISO at ArchiveSpec.ISOPath to the
// registry-node-agent pod scheduled on `targetNode`. Steps:
//
//  1. Resolve the target node's InternalIP.
//  2. Build a mTLS-enabled HTTP client trusting the registry CA and
//     presenting a fresh client cert signed by that same CA.
//  3. PUT the ISO in Content-Range chunks against
//     `https://<nodeIP>:5001/api/v1/uploads/<name>/<version>` until the
//     full file has been transferred.
func UploadArchiveISO(
	ctx context.Context,
	c client.Client,
	spec ArchiveSpec,
	targetNode string,
) error {
	caCertPEM, caKeyPEM, err := EnsureCA(ctx, c)
	if err != nil {
		return err
	}

	// Client cert CN is not verified beyond chain-of-trust; the RNA's
	// server side just requires a cert signed by the CA in caSecretRef.
	clientCertPEM, clientKeyPEM, err := NewClientCert(caCertPEM, caKeyPEM, "e2e-upload-client")
	if err != nil {
		return fmt.Errorf("client cert: %w", err)
	}
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return fmt.Errorf("unable to load CA into pool")
	}

	nodeIP, err := GetNodeInternalIP(ctx, c, targetNode)
	if err != nil {
		return err
	}

	// The RNA's server cert SAN covers its node IP (see
	// ReconcileRNAExternalServerCertificate in internal/controller/utils.go),
	// so standard hostname verification against nodeIP works.
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
	}

	httpClient := &http.Client{
		Timeout:   3 * time.Minute,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}

	endpoint := (&url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s:%d", nodeIP, rnaUploadPort),
		Path:   fmt.Sprintf("/api/v1/uploads/%s/%s", spec.Name, spec.Version),
	}).String()

	return uploadChunks(ctx, httpClient, endpoint, spec)
}

func uploadChunks(ctx context.Context, httpClient *http.Client, endpoint string, spec ArchiveSpec) error {
	fi, err := os.Stat(spec.ISOPath)
	if err != nil {
		return err
	}
	fileSize := fi.Size()
	if fileSize == 0 {
		return fmt.Errorf("ISO %s is empty", spec.ISOPath)
	}

	f, err := os.Open(spec.ISOPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, uploadChunkSize)
	var offset int64
	for offset < fileSize {
		n, readErr := io.ReadFull(f, buf)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return fmt.Errorf("read ISO: %w", readErr)
		}
		if n == 0 {
			break
		}
		chunk := buf[:n]
		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, offset+int64(n)-1, fileSize)

		if err := putChunk(ctx, httpClient, endpoint, contentRange, chunk); err != nil {
			return fmt.Errorf("chunk at offset %d: %w", offset, err)
		}
		offset += int64(n)
	}
	return nil
}

func putChunk(ctx context.Context, httpClient *http.Client, endpoint, contentRange string, chunk []byte) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(chunk))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Range", contentRange)
		req.Header.Set("Content-Type", "application/octet-stream")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return lastErr
}
