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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const (
	// Label names of the MirrorConfig metrics.
	labelNamespace = "mirrorconfig_namespace"
	labelName      = "name"
	labelReason    = "reason"

	// conditionReady is the type of the MirrorConfig condition reported by the metric.
	conditionReady = "Ready"

	// ReasonUnknown is the "reason" label value when the MirrorConfig has no Ready condition yet.
	ReasonUnknown = "Unknown"
)

// MirrorConfigReadyDesc describes the readiness of a MirrorConfig and the reason of its state.
var MirrorConfigReadyDesc = prometheus.NewDesc(
	"registry_mirrorconfig_ready",
	"Whether the MirrorConfig is deployed (1) or not (0), with the reason of its Ready condition.",
	[]string{labelNamespace, labelName, labelReason},
	nil,
)

// MirrorConfigCollector is a Prometheus collector that reports metrics from the MirrorConfig status.
type MirrorConfigCollector struct {
	reader client.Reader
}

// NewMirrorConfigCollector returns a MirrorConfigCollector that reads MirrorConfigs from the given reader.
func NewMirrorConfigCollector(reader client.Reader) *MirrorConfigCollector {
	return &MirrorConfigCollector{reader: reader}
}

// Describe sends the descriptors of the metrics reported by the collector.
func (c *MirrorConfigCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- MirrorConfigReadyDesc
}

// Collect sends the current metric values, read from the MirrorConfig status.
func (c *MirrorConfigCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	mirrorConfigs := &metalk8sv1alpha1.MirrorConfigList{}
	if err := c.reader.List(ctx, mirrorConfigs); err != nil {
		// An invalid metric would make the whole /metrics endpoint fail, so only log.
		logf.Log.WithName("metrics").Error(err, "unable to list MirrorConfigs")
		return
	}

	for i := range mirrorConfigs.Items {
		mc := &mirrorConfigs.Items[i]
		ready, reason := false, ReasonUnknown
		if cond := meta.FindStatusCondition(mc.Status.Conditions, conditionReady); cond != nil {
			ready, reason = cond.Status == metav1.ConditionTrue, cond.Reason
		}
		ch <- prometheus.MustNewConstMetric(MirrorConfigReadyDesc, prometheus.GaugeValue,
			boolToFloat(ready), mc.Namespace, mc.Name, reason)
	}
}
