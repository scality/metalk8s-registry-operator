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

package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("safeNodeName", func() {
	It("keeps short names without dots unchanged", func() {
		Expect(safeNodeName("node-1")).To(Equal("node-1"))
		Expect(safeNodeName("bootstrap")).To(Equal("bootstrap"))
	})

	It("sanitizes and hashes FQDN node names", func() {
		token := safeNodeName("ip-172-30-200-101.eu-north-1.compute.internal")
		Expect(token).NotTo(ContainSubstring("."))
		Expect(len(token)).To(BeNumerically("<=", 23))
		Expect(token).To(HavePrefix("ip-172-30-200"))
	})

	It("truncates and hashes long names", func() {
		token := safeNodeName(strings.Repeat("a", 100))
		Expect(len(token)).To(BeNumerically("<=", 23))
	})

	It("is deterministic", func() {
		name := "ip-172-30-200-101.eu-north-1.compute.internal"
		Expect(safeNodeName(name)).To(Equal(safeNodeName(name)))
	})

	It("does not collide on sanitization", func() {
		Expect(safeNodeName(strings.Repeat("a", 30) + ".b")).NotTo(
			Equal(safeNodeName(strings.Repeat("a", 30) + "-b")),
		)
	})

	It("uses a fixed-width hash suffix", func() {
		// This name hashes to less than 8 hex chars without zero-padding.
		Expect(getHash32Name("ip-172-30-200-19.eu-north-1.compute.internal")).To(HaveLen(8))
	})

	It("produces same-length tokens for same-shaped node names", func() {
		Expect(safeNodeName("ip-172-30-200-19.eu-north-1.compute.internal")).To(
			HaveLen(len(safeNodeName("ip-172-30-200-101.eu-north-1.compute.internal"))),
		)
	})
})
