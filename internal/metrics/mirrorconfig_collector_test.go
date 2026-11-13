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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const mirrorConfigReadyMetric = "registry_mirrorconfig_ready"

func newMirrorConfig(namespace, name string, conditions ...metav1.Condition) *metalk8sv1alpha1.MirrorConfig {
	return &metalk8sv1alpha1.MirrorConfig{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Status:     metalk8sv1alpha1.MirrorConfigStatus{Conditions: conditions},
	}
}

func readyCondition(status metav1.ConditionStatus, reason string) metav1.Condition {
	return metav1.Condition{Type: conditionReady, Status: status, Reason: reason}
}

var _ = Describe("MirrorConfigCollector", func() {
	It("reports the readiness and the reason of each MirrorConfig", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(
			newMirrorConfig("ns-1", "mirror-a",
				readyCondition(metav1.ConditionTrue, metalk8sv1alpha1.MirrorConfigReasonConfigMapRendered)),
			newMirrorConfig("ns-1", "mirror-b",
				readyCondition(metav1.ConditionFalse, metalk8sv1alpha1.MirrorConfigReasonMultipleRegistries)),
			newMirrorConfig("ns-2", "mirror-a",
				readyCondition(metav1.ConditionFalse, metalk8sv1alpha1.MirrorConfigReasonRegistryNotReady)),
		).Build()

		expected := `
# HELP registry_mirrorconfig_ready Whether the MirrorConfig is deployed (1) or not (0), with the reason of its Ready condition.
# TYPE registry_mirrorconfig_ready gauge
registry_mirrorconfig_ready{mirrorconfig_namespace="ns-1",name="mirror-a",reason="ConfigMapRendered"} 1
registry_mirrorconfig_ready{mirrorconfig_namespace="ns-2",name="mirror-a",reason="RegistryNotReady"} 0
registry_mirrorconfig_ready{mirrorconfig_namespace="ns-1",name="mirror-b",reason="MultipleRegistries"} 0
`
		Expect(testutil.CollectAndCompare(NewMirrorConfigCollector(c), strings.NewReader(expected),
			mirrorConfigReadyMetric)).To(Succeed())
	})

	It("reports not ready with the Unknown reason when the MirrorConfig has no Ready condition", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(newMirrorConfig("ns-1", "mirror-a")).Build()

		expected := `
# HELP registry_mirrorconfig_ready Whether the MirrorConfig is deployed (1) or not (0), with the reason of its Ready condition.
# TYPE registry_mirrorconfig_ready gauge
registry_mirrorconfig_ready{mirrorconfig_namespace="ns-1",name="mirror-a",reason="Unknown"} 0
`
		Expect(testutil.CollectAndCompare(NewMirrorConfigCollector(c), strings.NewReader(expected),
			mirrorConfigReadyMetric)).To(Succeed())
	})

	It("reports nothing when there is no MirrorConfig", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).Build()
		Expect(testutil.CollectAndCount(NewMirrorConfigCollector(c))).To(Equal(0))
	})

	It("reports nothing when the MirrorConfigs cannot be listed", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).Build()
		Expect(testutil.CollectAndCount(NewMirrorConfigCollector(c))).To(Equal(0))
	})
})
