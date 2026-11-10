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

package metrics

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const (
	nodeProcessReadyMetric = "registry_node_process_ready"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(metalk8sv1alpha1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

func newRegistry(selectedNodes []string, statusPerNode map[string]metalk8sv1alpha1.NodeStatus) *metalk8sv1alpha1.Registry {
	return &metalk8sv1alpha1.Registry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-registry"},
		Status: metalk8sv1alpha1.RegistryStatus{
			SelectedNodes: selectedNodes,
			StatusPerNode: statusPerNode,
		},
	}
}

var _ = Describe("RegistryCollector", func() {
	It("reports the readiness of each process on each node", func() {
		registry := newRegistry(
			[]string{"node-1", "node-2"},
			map[string]metalk8sv1alpha1.NodeStatus{
				"node-1": {
					Server: metalk8sv1alpha1.ProcessStatus{Available: true, Ready: true},
					Agent:  metalk8sv1alpha1.ProcessStatus{Available: true, Ready: true},
				},
				"node-2": {
					Server: metalk8sv1alpha1.ProcessStatus{Available: true, Ready: false},
					Agent:  metalk8sv1alpha1.ProcessStatus{Available: true, Ready: true},
				},
			},
		)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(registry).Build()

		expected := `
# HELP registry_node_process_ready Whether the registry process (server or agent) is ready on a selected node (1) or not (0).
# TYPE registry_node_process_ready gauge
registry_node_process_ready{node="node-1",process="agent",registry="my-registry"} 1
registry_node_process_ready{node="node-1",process="server",registry="my-registry"} 1
registry_node_process_ready{node="node-2",process="agent",registry="my-registry"} 1
registry_node_process_ready{node="node-2",process="server",registry="my-registry"} 0
`
		Expect(testutil.CollectAndCompare(NewRegistryCollector(c), strings.NewReader(expected), nodeProcessReadyMetric)).To(Succeed())
	})

	It("reports nothing when there is no Registry", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).Build()
		Expect(testutil.CollectAndCount(NewRegistryCollector(c))).To(Equal(0))
	})

	It("reports nothing per node when the Registry has no selected node", func() {
		registry := newRegistry(nil, map[string]metalk8sv1alpha1.NodeStatus{
			"node-1": {Server: metalk8sv1alpha1.ProcessStatus{Available: true, Ready: true}},
		})
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(registry).Build()
		Expect(testutil.CollectAndCount(NewRegistryCollector(c), nodeProcessReadyMetric)).To(Equal(0))
	})

	It("reports nothing when the Registries cannot be listed", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).Build()
		Expect(testutil.CollectAndCount(NewRegistryCollector(c))).To(Equal(0))
	})
})
