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

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const (
	// Label names of the SolutionArchive metrics.
	labelSolutionName    = "solution_name"
	labelSolutionVersion = "solution_version"
)

// SolutionArchiveServedReplicasDesc describes the number of served replicas of a SolutionArchive.
var SolutionArchiveServedReplicasDesc = prometheus.NewDesc(
	"registry_soa_served_replicas",
	"Number of NodeSolutionArchives of the SolutionArchive in Served status.",
	[]string{labelSolutionName, labelSolutionVersion},
	nil,
)

// SolutionArchiveCollector is a Prometheus collector that reports metrics from the SolutionArchive status.
type SolutionArchiveCollector struct {
	reader client.Reader
}

// NewSolutionArchiveCollector returns a SolutionArchiveCollector that reads SolutionArchives from the given reader.
func NewSolutionArchiveCollector(reader client.Reader) *SolutionArchiveCollector {
	return &SolutionArchiveCollector{reader: reader}
}

// Describe sends the descriptors of the metrics reported by the collector.
func (c *SolutionArchiveCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- SolutionArchiveServedReplicasDesc
}

// Collect sends the current metric values, read from the SolutionArchive status.
func (c *SolutionArchiveCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	solutionArchives := &metalk8sv1alpha1.SolutionArchiveList{}
	if err := c.reader.List(ctx, solutionArchives); err != nil {
		// An invalid metric would make the whole /metrics endpoint fail, so only log.
		logf.Log.WithName("metrics").Error(err, "unable to list SolutionArchives")
		return
	}

	for i := range solutionArchives.Items {
		soa := &solutionArchives.Items[i]
		ch <- prometheus.MustNewConstMetric(SolutionArchiveServedReplicasDesc, prometheus.GaugeValue,
			float64(ptr.Deref(soa.Status.ServedReplicas, 0)), soa.Spec.Name, soa.Spec.Version)
	}
}
