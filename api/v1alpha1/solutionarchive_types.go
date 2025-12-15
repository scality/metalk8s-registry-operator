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
	rnav1alpha1 "github.com/scality/metalk8s-registry-node-agent/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type NodeSolutionArchiveStatus struct {
	// True, when the NodeSolutionArchive is in Served status
	Served bool `json:"served,omitempty"`
}

// SolutionArchiveStatus defines the observed state of SolutionArchive.
type SolutionArchiveStatus struct {
	// True, when, at least, one NodeSolutionArchive in Served status
	Served *bool `json:"served,omitempty"`
	// True, when all NodeSolutionArchive in Served status
	Replicated *bool `json:"replicated,omitempty"`
	// Number of NodeSolutionArchive in Served status
	ServedReplicas *int `json:"servedReplicas,omitempty"`
	// Expected number of NodeSolutionArchive in Served status
	TargetReplicas *int `json:"targetReplicas,omitempty"`
	// List of NodeSolutionArchive names
	NodeSolutionArchives []string `json:"nodeSolutionArchives,omitempty"`
	// Status of each NodeSolutionArchive
	StatusPerNodeSolutionArchive map[string]NodeSolutionArchiveStatus `json:"statusPerNodeSolutionArchive,omitempty"`
	Conditions                   []metav1.Condition                   `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// +kubebuilder:printcolumn:name="Solution",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version"
// +kubebuilder:printcolumn:name="Served",type="boolean",JSONPath=".status.served",priority=1
// +kubebuilder:printcolumn:name="Replicated",type="boolean",JSONPath=".status.replicated"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.servedReplicas"
// +kubebuilder:printcolumn:name="Target",type="integer",JSONPath=".status.targetReplicas"
// +kubebuilder:printcolumn:name="Node Solution Archives",type="string",JSONPath=".status.nodeSolutionArchives",priority=1
// SolutionArchive is the Schema for the solutionarchives API.
type SolutionArchive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   rnav1alpha1.SolutionArchiveSpec `json:"spec,omitempty"`
	Status SolutionArchiveStatus           `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SolutionArchiveList contains a list of SolutionArchive.
type SolutionArchiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SolutionArchive `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SolutionArchive{}, &SolutionArchiveList{})
}

func (solutionArchive *SolutionArchive) InitStatus() {
	if solutionArchive.Status.Served == nil {
		solutionArchive.Status.Served = ptr.To(false)
	}
	if solutionArchive.Status.Replicated == nil {
		solutionArchive.Status.Replicated = ptr.To(false)
	}
	if solutionArchive.Status.ServedReplicas == nil {
		solutionArchive.Status.ServedReplicas = ptr.To(0)
	}
	if solutionArchive.Status.TargetReplicas == nil {
		solutionArchive.Status.TargetReplicas = ptr.To(0)
	}
	if solutionArchive.Status.NodeSolutionArchives == nil {
		solutionArchive.Status.NodeSolutionArchives = []string{}
	}
}

func (solutionArchive *SolutionArchive) ResetStatus() {
	solutionArchive.Status.Served = ptr.To(false)
	solutionArchive.Status.Replicated = ptr.To(false)
	solutionArchive.Status.ServedReplicas = ptr.To(0)
	solutionArchive.Status.TargetReplicas = ptr.To(0)
	solutionArchive.Status.NodeSolutionArchives = []string{}
}
