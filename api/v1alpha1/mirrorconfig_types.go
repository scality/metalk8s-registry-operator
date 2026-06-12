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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MirrorConfigSpec defines the desired state of MirrorConfig.
type MirrorConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MirrorConfig. Edit mirrorconfig_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// MirrorConfigStatus defines the observed state of MirrorConfig.
type MirrorConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MirrorConfig is the Schema for the mirrorconfigs API.
type MirrorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MirrorConfigSpec   `json:"spec,omitempty"`
	Status MirrorConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MirrorConfigList contains a list of MirrorConfig.
type MirrorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MirrorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MirrorConfig{}, &MirrorConfigList{})
}
