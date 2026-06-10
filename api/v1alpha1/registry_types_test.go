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
	"k8s.io/utils/ptr"
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

	Describe("GetMirrorPropagationImage", func() {
		It("Should return the default image when mirrorPropagation is nil", func() {
			r := &Registry{}
			Expect(r.GetMirrorPropagationImage().GetImage()).To(Equal("ghcr.io/scality/file-reflector:v0.1.0"))
		})

		It("Should return the custom image when set", func() {
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{
				Image: &ImageSpec{Registry: "registry.example.com", Name: "custom", Tag: ptr.To("v1.2.3")},
			}}}
			Expect(r.GetMirrorPropagationImage().GetImage()).To(Equal("registry.example.com/custom:v1.2.3"))
		})

		It("Should default the tag to latest without mutating the spec", func() {
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{
				Image: &ImageSpec{Registry: "registry.example.com", Name: "custom"},
			}}}
			Expect(r.GetMirrorPropagationImage().GetImage()).To(Equal("registry.example.com/custom:latest"))
			Expect(r.Spec.MirrorPropagation.Image.Tag).To(BeNil())
		})
	})

	Describe("GetContainerdConfigPath", func() {
		It("Should return the default path when mirrorPropagation is nil", func() {
			r := &Registry{}
			Expect(r.GetContainerdConfigPath()).To(Equal("/etc/containerd/certs.d"))
		})

		It("Should return the default path when the section is set but the path is empty", func() {
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{Enabled: true}}}
			Expect(r.GetContainerdConfigPath()).To(Equal("/etc/containerd/certs.d"))
		})

		It("Should return the custom path when set", func() {
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{
				ContainerdConfigPath: "/var/lib/containerd/certs.d",
			}}}
			Expect(r.GetContainerdConfigPath()).To(Equal("/var/lib/containerd/certs.d"))
		})
	})

	Describe("GetMirrorPropagationNodeSelector", func() {
		It("Should return the default selector when mirrorPropagation is nil", func() {
			r := &Registry{}
			Expect(r.GetMirrorPropagationNodeSelector()).To(Equal(map[string]string{"kubernetes.io/os": "linux"}))
		})

		It("Should return the custom selector when set", func() {
			r := &Registry{Spec: RegistrySpec{MirrorPropagation: &MirrorPropagationSpec{
				NodeSelector: map[string]string{"kubernetes.io/arch": "amd64"},
			}}}
			Expect(r.GetMirrorPropagationNodeSelector()).To(Equal(map[string]string{"kubernetes.io/arch": "amd64"}))
		})
	})
})
