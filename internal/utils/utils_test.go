/*
Copyright 2025 Scality.

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

package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GenerateContainerdHostsToml", func() {
	It("returns an empty string when there are no hosts", func() {
		Expect(GenerateContainerdHostsToml(nil)).To(Equal(""))
	})

	It("renders a single host block", func() {
		Expect(GenerateContainerdHostsToml([]string{"https://10.96.0.1:5000"})).To(Equal(
			`[host."https://10.96.0.1:5000"]
  capabilities = ["pull", "resolve"]
  ca = "ca.crt"
`))
	})

	It("preserves order: ClusterIP first, then node IPs", func() {
		got := GenerateContainerdHostsToml([]string{
			"https://10.96.0.1:5000",
			"https://10.0.0.1:5000",
			"https://10.0.0.2:5000",
		})
		Expect(got).To(Equal(
			`[host."https://10.96.0.1:5000"]
  capabilities = ["pull", "resolve"]
  ca = "ca.crt"
[host."https://10.0.0.1:5000"]
  capabilities = ["pull", "resolve"]
  ca = "ca.crt"
[host."https://10.0.0.2:5000"]
  capabilities = ["pull", "resolve"]
  ca = "ca.crt"
`))
	})
})
