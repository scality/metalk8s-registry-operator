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

// Package e2e exercises the metalk8s-registry-operator against an existing
// Kubernetes cluster on which the operator has already been deployed.
//
// Unlike the integration suite under test/integration, these tests do not use
// envtest and do not start a controller manager in-process: they target a
// real cluster reached via the standard kubeconfig loading rules (i.e.
// $KUBECONFIG, or ~/.kube/config as a fallback).
package e2e

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

// Shared across all specs in this package; populated by BeforeSuite.
var (
	k8sClient client.Client
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("loading the kubeconfig")
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	Expect(err).NotTo(HaveOccurred(),
		"failed to load kubeconfig; set $KUBECONFIG to point at the test cluster")

	By("registering the operator + peer API schemes")
	Expect(metalk8sv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiextv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(rnav1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(cmapi.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
})
