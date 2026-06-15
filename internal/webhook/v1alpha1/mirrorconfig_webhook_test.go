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

	metalk8sv1alpha1 "github.com/scality/metalk8s-registry-operator/api/v1alpha1"
	// TODO (user): Add any additional imports if needed
)

var _ = Describe("MirrorConfig Webhook", func() {
	var (
		obj       *metalk8sv1alpha1.MirrorConfig
		oldObj    *metalk8sv1alpha1.MirrorConfig
		validator MirrorConfigCustomValidator
	)

	BeforeEach(func() {
		obj = &metalk8sv1alpha1.MirrorConfig{}
		oldObj = &metalk8sv1alpha1.MirrorConfig{}
		validator = MirrorConfigCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating MirrorConfig under Validating Webhook", func() {
		makeRegistries := func(prefixes ...string) []metalk8sv1alpha1.MirrorRegistry {
			registries := make([]metalk8sv1alpha1.MirrorRegistry, 0, len(prefixes))
			for _, prefix := range prefixes {
				registries = append(registries, metalk8sv1alpha1.MirrorRegistry{Prefix: prefix})
			}
			return registries
		}

		It("Should admit creation with unique prefixes", func() {
			obj.Spec.Registries = makeRegistries("docker.io", "ghcr.io")
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny creation with duplicate prefixes", func() {
			obj.Spec.Registries = makeRegistries("docker.io", "docker.io")
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring("duplicate registry prefix")))
		})

		It("Should deny update introducing duplicate prefixes", func() {
			oldObj.Spec.Registries = makeRegistries("docker.io")
			obj.Spec.Registries = makeRegistries("ghcr.io", "ghcr.io")
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(MatchError(ContainSubstring("duplicate registry prefix")))
		})

		It("Should admit an empty registries list", func() {
			obj.Spec.Registries = nil
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
