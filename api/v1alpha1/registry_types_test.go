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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Registry", func() {
	Describe("IsMirrorPropagationEnabled", func() {
		It("Should return true when mirrorPropagation section is nil (default)", func() {
			By("creating a Registry with no mirrorPropagation section")
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: nil}}

			By("checking that IsMirrorPropagationEnabled returns true")
			Expect(r.IsMirrorPropagationEnabled()).To(BeTrue())
		})

		It("Should return true when mirrorPropagation is explicitly enabled", func() {
			By("creating a Registry with mirrorPropagation explicitly enabled")
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{Enabled: true}}}

			By("checking that IsMirrorPropagationEnabled returns true")
			Expect(r.IsMirrorPropagationEnabled()).To(BeTrue())
		})

		It("Should return false when mirrorPropagation is explicitly disabled", func() {
			By("creating a Registry with mirrorPropagation explicitly disabled")
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{Enabled: false}}}

			By("checking that IsMirrorPropagationEnabled returns false")
			Expect(r.IsMirrorPropagationEnabled()).To(BeFalse())
		})
	})
})
