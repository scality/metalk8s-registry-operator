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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
)

const soaServedReplicasMetric = "registry_soa_served_replicas"

func newSolutionArchive(name, version string, servedReplicas *int) *metalk8sv1alpha1.SolutionArchive {
	soa := &metalk8sv1alpha1.SolutionArchive{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-" + version},
		Status:     metalk8sv1alpha1.SolutionArchiveStatus{ServedReplicas: servedReplicas},
	}
	soa.Spec.Name = name
	soa.Spec.Version = version
	return soa
}

var _ = Describe("SolutionArchiveCollector", func() {
	It("reports the served replicas of each SolutionArchive", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(
			newSolutionArchive("my-solution", "1.0.0", ptr.To(3)),
			newSolutionArchive("my-solution", "2.0.0", ptr.To(1)),
		).Build()

		expected := `
# HELP registry_soa_served_replicas Number of NodeSolutionArchives of the SolutionArchive in Served status.
# TYPE registry_soa_served_replicas gauge
registry_soa_served_replicas{solution_name="my-solution",solution_version="1.0.0"} 3
registry_soa_served_replicas{solution_name="my-solution",solution_version="2.0.0"} 1
`
		Expect(testutil.CollectAndCompare(NewSolutionArchiveCollector(c), strings.NewReader(expected),
			soaServedReplicasMetric)).To(Succeed())
	})

	It("reports zero served replicas when the SolutionArchive status is empty", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(
			newSolutionArchive("my-solution", "1.0.0", nil),
		).Build()

		expected := `
# HELP registry_soa_served_replicas Number of NodeSolutionArchives of the SolutionArchive in Served status.
# TYPE registry_soa_served_replicas gauge
registry_soa_served_replicas{solution_name="my-solution",solution_version="1.0.0"} 0
`
		Expect(testutil.CollectAndCompare(NewSolutionArchiveCollector(c), strings.NewReader(expected),
			soaServedReplicasMetric)).To(Succeed())
	})

	It("reports nothing when there is no SolutionArchive", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).Build()
		Expect(testutil.CollectAndCount(NewSolutionArchiveCollector(c))).To(Equal(0))
	})

	It("reports nothing when the SolutionArchives cannot be listed", func() {
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).Build()
		Expect(testutil.CollectAndCount(NewSolutionArchiveCollector(c))).To(Equal(0))
	})
})
