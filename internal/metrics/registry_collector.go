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

// Package metrics exposes the operator business metrics to Prometheus.
package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const (
	// ProcessServer is the "process" label value for the registry server.
	ProcessServer = "server"
	// ProcessAgent is the "process" label value for the registry node agent.
	ProcessAgent = "agent"

	// Label names shared by the registry metrics.
	labelRegistry = "registry"
	labelNode     = "node"
	labelProcess  = "process"

	// collectTimeout limits the time spent to read resources during a scrape.
	collectTimeout = 5 * time.Second
)

// RegistryNodeProcessReadyDesc describes the readiness of a registry process on a selected node.
var RegistryNodeProcessReadyDesc = prometheus.NewDesc(
	"registry_node_process_ready",
	"Whether the registry process (server or agent) is ready on a selected node (1) or not (0).",
	[]string{labelRegistry, labelNode, labelProcess},
	nil,
)

// RegistryNodeProcessAvailableDesc describes the availability of a registry process on a selected node.
var RegistryNodeProcessAvailableDesc = prometheus.NewDesc(
	"registry_node_process_available",
	"Whether the registry process (server or agent) is available on a selected node (1) or not (0).",
	[]string{labelRegistry, labelNode, labelProcess},
	nil,
)

// RegistrySelectedNodesDesc describes the number of nodes selected for a registry.
var RegistrySelectedNodesDesc = prometheus.NewDesc(
	"registry_selected_nodes",
	"Number of nodes selected for the registry, which is the number of expected replicas.",
	[]string{labelRegistry},
	nil,
)

// RegistryCollector is a Prometheus collector that reports metrics from the Registry status.
type RegistryCollector struct {
	reader client.Reader
}

// NewRegistryCollector returns a RegistryCollector that reads Registries from the given reader.
func NewRegistryCollector(reader client.Reader) *RegistryCollector {
	return &RegistryCollector{reader: reader}
}

// Describe sends the descriptors of the metrics reported by the collector.
func (c *RegistryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- RegistryNodeProcessReadyDesc
	ch <- RegistryNodeProcessAvailableDesc
	ch <- RegistrySelectedNodesDesc
}

// Collect sends the current metric values, read from the Registry status.
func (c *RegistryCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	registries := &metalk8sv1alpha1.RegistryList{}
	if err := c.reader.List(ctx, registries); err != nil {
		// An invalid metric would make the whole /metrics endpoint fail, so only log.
		logf.Log.WithName("metrics").Error(err, "unable to list Registries")
		return
	}

	for i := range registries.Items {
		registry := &registries.Items[i]
		ch <- prometheus.MustNewConstMetric(RegistrySelectedNodesDesc, prometheus.GaugeValue,
			float64(len(registry.Status.SelectedNodes)), registry.Name)

		for _, node := range registry.Status.SelectedNodes {
			// A selected node with no status yet reports 0 for every process.
			status := registry.Status.StatusPerNode[node]
			ch <- prometheus.MustNewConstMetric(RegistryNodeProcessReadyDesc, prometheus.GaugeValue,
				boolToFloat(status.Server.Ready), registry.Name, node, ProcessServer)
			ch <- prometheus.MustNewConstMetric(RegistryNodeProcessReadyDesc, prometheus.GaugeValue,
				boolToFloat(status.Agent.Ready), registry.Name, node, ProcessAgent)
			ch <- prometheus.MustNewConstMetric(RegistryNodeProcessAvailableDesc, prometheus.GaugeValue,
				boolToFloat(status.Server.Available), registry.Name, node, ProcessServer)
			ch <- prometheus.MustNewConstMetric(RegistryNodeProcessAvailableDesc, prometheus.GaugeValue,
				boolToFloat(status.Agent.Available), registry.Name, node, ProcessAgent)
		}
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
