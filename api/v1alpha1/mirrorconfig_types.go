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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MirrorConfigReasonConfigMapRendered  = "ConfigMapRendered"
	MirrorConfigReasonRegistryNotReady   = "RegistryNotReady"
	MirrorConfigReasonMultipleRegistries = "MultipleRegistries"
)

type MirrorRegistry struct {
	// Prefix of the upstream registry to mirror (e.g. "docker.io").
	// +kubebuilder:validation:MinLength=1
	Prefix string `json:"prefix"`
}

// MirrorConfigSpec defines the desired state of MirrorConfig.
type MirrorConfigSpec struct {
	// Registries lists the upstream registries the workload pulls through the mirror.
	// +kubebuilder:validation:Optional
	Registries []MirrorRegistry `json:"registries,omitempty"`
}

// MirrorConfigStatus defines the observed state of MirrorConfig.
type MirrorConfigStatus struct {
	// CASecretRef references the Secret the registry CA was read from.
	CASecretRef *corev1.SecretReference `json:"caSecretRef,omitempty"`
	// ObservedRegistries lists the prefixes rendered into the ConfigMap.
	ObservedRegistries []string `json:"observedRegistries,omitempty"`
	// Conditions of the MirrorConfig.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

// SetReady sets the Ready condition of the MirrorConfig.
func (m *MirrorConfig) SetReady(ready bool, reason string, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: m.Generation,
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
	}
	meta.SetStatusCondition(&m.Status.Conditions, condition)
}
