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

// Package v1alpha1 contains API Schema definitions for the metalk8s v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=metalk8s.scality.com
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "metalk8s.scality.com", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &schemeBuilder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// schemeBuilder mirrors sigs.k8s.io/controller-runtime/pkg/scheme.Builder,
// which is deprecated in controller-runtime v0.24 because api packages should
// keep their imports minimal. Inlining preserves the SchemeBuilder.Register
// call sites in the per-type init() blocks without pulling in the deprecated
// package.
type schemeBuilder struct {
	GroupVersion schema.GroupVersion
	runtime.SchemeBuilder
}

// Register adds one or more objects to the SchemeBuilder so they can be added
// to a Scheme.
func (b *schemeBuilder) Register(objects ...runtime.Object) *schemeBuilder {
	b.SchemeBuilder.Register(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(b.GroupVersion, objects...)
		metav1.AddToGroupVersion(scheme, b.GroupVersion)
		return nil
	})
	return b
}
