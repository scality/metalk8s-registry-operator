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
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	RegistryServerImageRegistry    = "ghcr.io/scality"
	RegistryServerImageName        = "metalk8s-registry-server"
	RegistryServerImageTag         = "v1.0.0"
	RegistryNodeAgentImageRegistry = "ghcr.io/scality"
	RegistryNodeAgentImageName     = "metalk8s-registry-node-agent"
	RegistryNodeAgentImageTag      = "v0.0.1-alpha.9"
	FileReflectorImageRegistry     = "ghcr.io/scality"
	FileReflectorImageName         = "file-reflector"
	FileReflectorImageTag          = "v0.2.0"
	DEFAULT_NAMESPACE              = "metalk8s-registry"
	DEFAULT_ARCHIVES_PATH          = "/srv/scality/metalk8s/archives"
	DEFAULT_SOLUTIONS_PATH         = "/srv/scality/metalk8s/solutions"
	DEFAULT_CONTAINERD_CONFIG_PATH = "/etc/containerd/certs.d"
)

type ImageSpec struct {
	// Registry URL, defaults to the component's default registry.
	// +kubebuilder:validation:Optional
	Registry string `json:"registry,omitempty"`
	// Name of the image, defaults to the component's default image name.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// Tag of the image, defaults to the component's default tag.
	// +kubebuilder:validation:Optional
	Tag *string `json:"tag,omitempty"`
	// PullPolicy of the image.
	// +kubebuilder:validation:Optional
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// PullSecrets is an optional list of references to secrets
	// to use for pulling the image.
	// +kubebuilder:validation:Optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

type RegistryServerSpec struct {
	// CertificateIssuerRef is a reference to the cert-manager Issuer or ClusterIssuer
	// that will generate the Certificate used by the registry server on its API endpoint.
	CertificateIssuerRef cmmetav1.ObjectReference `json:"certificateIssuerRef"`
	// Image is the specification of the registry server image.
	// +kubebuilder:validation:Optional
	Image *ImageSpec `json:"image,omitempty"`
}

type AuthenticationSpec struct {
	// mTLS authentication mechanism.
	MTLS MTLSAuthenticationSpec `json:"mtls"`
}

type MTLSAuthenticationSpec struct {
	// CASecretRef is a reference to the secret containing the CA certificate
	// used to generate the certificates used for mTLS authentication.
	CASecretRef corev1.SecretReference `json:"caSecretRef"`
}

type RegistryNodeAgentSpec struct {
	// CertificateIssuerRef is a reference to the cert-manager Issuer or ClusterIssuer
	// that will generate the Certificate used by the registry node agent on its upload API endpoint.
	CertificateIssuerRef cmmetav1.ObjectReference `json:"certificateIssuerRef"`
	// Authentication is the specification of the registry node agent authentication mechanism
	// used to allow SolutionArchive distribution between registry node agents.
	Authentication AuthenticationSpec `json:"authentication"`
	// Image is the specification of the registry node agent image.
	// +kubebuilder:validation:Optional
	Image *ImageSpec `json:"image,omitempty"`
}

type MirrorPropagationSpec struct {
	// Enabled controls whether the mirror config propagation is active.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	// Image is the specification of the file-reflector image.
	// +kubebuilder:validation:Optional
	Image *ImageSpec `json:"image,omitempty"`
	// ContainerdConfigPath is the containerd mirror config path on the host,
	// defaults to "/etc/containerd/certs.d".
	// +kubebuilder:default="/etc/containerd/certs.d"
	// +kubebuilder:validation:Optional
	ContainerdConfigPath string `json:"containerdConfigPath,omitempty"`
	// NodeSelector for the sync DaemonSet pods.
	// Defaults to {"kubernetes.io/os": "linux"}.
	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Tolerations for the sync DaemonSet pods.
	// +kubebuilder:validation:Optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// IgnorePaths is a list of paths in the target directory that should not be
	// managed by the file-reflector (e.g. legacy registry config managed externally).
	// +kubebuilder:validation:Optional
	IgnorePaths []string `json:"ignorePaths,omitempty"`
}

// RegistrySpec defines the desired state of Registry.
type RegistrySpec struct {
	// Log level for Registry Node Agent and Registry Server, defaults to "info"
	// +kubebuilder:default=info
	// +kubebuilder:validation:Optional
	LogLevel *string `json:"logLevel,omitempty"`
	// HostPath where to store ISO files, defaults to "/srv/scality/metalk8s/archives"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	ArchivesPath *string `json:"archivesPath,omitempty"`
	// HostPath where to mount ISO files, defaults to "/srv/scality/metalk8s/solutions"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	SolutionsPath *string `json:"solutionsPath,omitempty"`
	// Namespace where the registry resources are deployed, defaults to "metalk8s-registry-system".
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:Optional
	Namespace *string `json:"namespace,omitempty"`
	// NodeSelector is a selector which must be true for the registry to fit on a node.
	// Selector which must match a node's labels for the registry to be scheduled on that node.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector"`
	// Server is the specification of the registry server.
	Server RegistryServerSpec `json:"server"`
	// Agent is the specification of the registry node agent.
	Agent RegistryNodeAgentSpec `json:"agent"`
	// MirrorPropagation controls generation of the containerd registry mirror ConfigMap.
	// +kubebuilder:validation:Optional
	MirrorPropagation *MirrorPropagationSpec `json:"mirrorPropagation,omitempty"`
}

type ProcessStatus struct {
	// Availability of the process on the related node.
	Available bool `json:"available"`
	// Readiness of the process on the related node.
	Ready bool `json:"ready"`
}

type NodeStatus struct {
	// Status of the registry server on the related node.
	Server ProcessStatus `json:"server"`
	// Status of the registry node agent on the related node.
	Agent ProcessStatus `json:"agent"`
}

// RegistryStatus defines the observed state of Registry.
type RegistryStatus struct {
	// Availability of the registry.
	Available *bool `json:"available,omitempty"`
	// Readiness of the registry.
	Ready *bool `json:"ready,omitempty"`
	// Availability of the registry server.
	ServerAvailable *bool `json:"serverAvailable,omitempty"`
	// Readiness of the registry server.
	ServerReady *bool `json:"serverReady,omitempty"`
	// Availability of the registry node agent.
	AgentAvailable *bool `json:"agentAvailable,omitempty"`
	// Readiness of the registry node agent.
	AgentReady *bool `json:"agentReady,omitempty"`
	// Availability of the containerd mirror sync.
	MirrorSyncAvailable *bool `json:"mirrorSyncAvailable,omitempty"`
	// Readiness of the containerd mirror sync.
	MirrorSyncReady *bool `json:"mirrorSyncReady,omitempty"`
	// Number of replicas for NodeAgent and RegistryServer.
	Replicas *int `json:"replicas,omitempty"`
	// Number of ready replicas for RegistryServer.
	ReadyServerReplicas *int `json:"readyServerReplicas,omitempty"`
	// Number of ready replicas for RegistryNodeAgent.
	ReadyAgentReplicas *int `json:"readyAgentReplicas,omitempty"`
	// Selected nodes for the registry, based on NodeSelector.
	SelectedNodes []string `json:"selectedNodes,omitempty"`
	// ClusterIP at which the registry is reachable, load-balanced across the
	// registry server replicas by kube-proxy.
	ClusterIP string `json:"clusterIP,omitempty"`
	// NodeIPs at which the registry is reachable directly on each selected node.
	NodeIPs []string `json:"nodeIPs,omitempty"`
	// Status per node for the registry,
	// including availability and readiness of the registry server and node agent.
	StatusPerNode map[string]NodeStatus `json:"statusPerNode,omitempty"`
	// Conditions of the registry.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// +kubebuilder:printcolumn:name="Available",type="boolean",JSONPath=".status.available",priority=1
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="ClusterIP",type="string",JSONPath=".status.clusterIP"
// +kubebuilder:printcolumn:name="Server Replicas",type="integer",JSONPath=".status.readyServerReplicas",priority=1
// +kubebuilder:printcolumn:name="Agent Replicas",type="integer",JSONPath=".status.readyAgentReplicas",priority=1
// +kubebuilder:printcolumn:name="Selected Nodes",type="string",JSONPath=".status.selectedNodes",priority=1
// Registry is the Schema for the registries API.
type Registry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistrySpec   `json:"spec,omitempty"`
	Status RegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RegistryList contains a list of Registry.
type RegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Registry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Registry{}, &RegistryList{})
}

func (registry *Registry) InitStatus() {
	if registry.Status.Available == nil {
		registry.Status.Available = ptr.To(false)
	}
	if registry.Status.Ready == nil {
		registry.Status.Ready = ptr.To(false)
	}
	if registry.Status.ServerAvailable == nil {
		registry.Status.ServerAvailable = ptr.To(false)
	}
	if registry.Status.ServerReady == nil {
		registry.Status.ServerReady = ptr.To(false)
	}
	if registry.Status.AgentAvailable == nil {
		registry.Status.AgentAvailable = ptr.To(false)
	}
	if registry.Status.AgentReady == nil {
		registry.Status.AgentReady = ptr.To(false)
	}
	if registry.Status.MirrorSyncAvailable == nil {
		registry.Status.MirrorSyncAvailable = ptr.To(false)
	}
	if registry.Status.MirrorSyncReady == nil {
		registry.Status.MirrorSyncReady = ptr.To(false)
	}
	if registry.Status.Replicas == nil {
		registry.Status.Replicas = ptr.To(0)
	}
	if registry.Status.ReadyServerReplicas == nil {
		registry.Status.ReadyServerReplicas = ptr.To(0)
	}
	if registry.Status.ReadyAgentReplicas == nil {
		registry.Status.ReadyAgentReplicas = ptr.To(0)
	}
	if registry.Status.SelectedNodes == nil {
		registry.Status.SelectedNodes = []string{}
	}
	if registry.Status.NodeIPs == nil {
		registry.Status.NodeIPs = []string{}
	}
	if registry.Status.StatusPerNode == nil {
		registry.Status.StatusPerNode = make(map[string]NodeStatus)
	}
}

// defaultImageSpec fills the empty fields of the given image spec with the
// provided component defaults, creating the spec when nil.
//
//nolint:unparam // every component currently shares the same default registry
func defaultImageSpec(image *ImageSpec, registry string, name string, tag string) *ImageSpec {
	if image == nil {
		image = &ImageSpec{}
	}
	if image.Registry == "" {
		image.Registry = registry
	}
	if image.Name == "" {
		image.Name = name
	}
	if image.Tag == nil {
		image.Tag = ptr.To(tag)
	}
	return image
}

func (registry *Registry) WithDefaults() {
	registry.Spec.Agent.Image = defaultImageSpec(
		registry.Spec.Agent.Image,
		RegistryNodeAgentImageRegistry, RegistryNodeAgentImageName, RegistryNodeAgentImageTag,
	)
	registry.Spec.Server.Image = defaultImageSpec(
		registry.Spec.Server.Image,
		RegistryServerImageRegistry, RegistryServerImageName, RegistryServerImageTag,
	)
	if registry.Spec.MirrorPropagation != nil {
		registry.Spec.MirrorPropagation.Image = defaultImageSpec(
			registry.Spec.MirrorPropagation.Image,
			FileReflectorImageRegistry, FileReflectorImageName, FileReflectorImageTag,
		)
	}
}

func (is *ImageSpec) GetImage() string {
	registryImageName := ""
	if is.Registry != "" {
		registryImageName = is.Registry + "/"
	}
	return registryImageName + is.Name + ":" + *is.Tag
}

// GetRegistryNamespace returns the namespace of the registry, or its default value if not set.
// note: We cannot use GetNamespace name as it is already defined by Kubernetes.
func (r *Registry) GetRegistryNamespace() string {
	if r.Spec.Namespace != nil && *r.Spec.Namespace != "" {
		return *r.Spec.Namespace
	}
	return DEFAULT_NAMESPACE
}

// GetArchivesPath returns the archives path of the registry, or its default value if not set.
func (r *Registry) GetArchivesPath() string {
	if r.Spec.ArchivesPath != nil && *r.Spec.ArchivesPath != "" {
		return *r.Spec.ArchivesPath
	}
	return DEFAULT_ARCHIVES_PATH
}

// GetSolutionsPath returns the solutions path of the registry, or its default value if not set.
func (r *Registry) GetSolutionsPath() string {
	if r.Spec.SolutionsPath != nil && *r.Spec.SolutionsPath != "" {
		return *r.Spec.SolutionsPath
	}
	return DEFAULT_SOLUTIONS_PATH
}

// IsMirrorPropagationEnabled returns whether the containerd mirror ConfigMap should
// be generated. It defaults to true when the mirrorPropagation section is omitted.
func (r *Registry) IsMirrorPropagationEnabled() bool {
	return r.Spec.MirrorPropagation == nil || r.Spec.MirrorPropagation.Enabled
}

// GetMirrorPropagationImage returns the file-reflector image spec, with its
// empty fields filled with the defaults. The returned ImageSpec always has a
// non-nil Tag and is a copy (the spec is never mutated).
func (r *Registry) GetMirrorPropagationImage() *ImageSpec {
	var image *ImageSpec
	if r.Spec.MirrorPropagation != nil {
		image = r.Spec.MirrorPropagation.Image.DeepCopy()
	}
	return defaultImageSpec(
		image,
		FileReflectorImageRegistry, FileReflectorImageName, FileReflectorImageTag,
	)
}

// GetContainerdConfigPath returns the containerd certs.d path on the host,
// or its default value if not set.
func (r *Registry) GetContainerdConfigPath() string {
	if r.Spec.MirrorPropagation != nil && r.Spec.MirrorPropagation.ContainerdConfigPath != "" {
		return r.Spec.MirrorPropagation.ContainerdConfigPath
	}
	return DEFAULT_CONTAINERD_CONFIG_PATH
}

// GetMirrorPropagationNodeSelector returns the sync DaemonSet nodeSelector,
// or its default value if not set.
func (r *Registry) GetMirrorPropagationNodeSelector() map[string]string {
	if r.Spec.MirrorPropagation != nil && len(r.Spec.MirrorPropagation.NodeSelector) > 0 {
		return r.Spec.MirrorPropagation.NodeSelector
	}
	return map[string]string{"kubernetes.io/os": "linux"}
}

func (r *Registry) SetAvailable(available bool) {
	condition := metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryAvailable",
		Message:            "The registry is available.",
		ObservedGeneration: r.Generation,
	}
	if !available {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryUnavailable"
		condition.Message = "The registry is not available."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.Available = ptr.To(available)
}

func (r *Registry) SetReady(ready bool) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryReady",
		Message:            "The registry is ready.",
		ObservedGeneration: r.Generation,
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryNotReady"
		condition.Message = "The registry is not ready."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.Ready = ptr.To(ready)
}

func (r *Registry) SetServerAvailable(available bool) {
	condition := metav1.Condition{
		Type:               "ServerAvailable",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryServerAvailable",
		Message:            "The registry server is available.",
		ObservedGeneration: r.Generation,
	}
	if !available {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryServerNotAvailable"
		condition.Message = "The registry server is not available."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.ServerAvailable = ptr.To(available)
}

func (r *Registry) SetServerReady(ready bool) {
	condition := metav1.Condition{
		Type:               "ServerReady",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryServerReady",
		Message:            "The registry server is ready.",
		ObservedGeneration: r.Generation,
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryServerNotReady"
		condition.Message = "The registry server is not ready."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.ServerReady = ptr.To(ready)
}

func (r *Registry) SetAgentAvailable(available bool) {
	condition := metav1.Condition{
		Type:               "AgentAvailable",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryAgentAvailable",
		Message:            "The registry agent is available.",
		ObservedGeneration: r.Generation,
	}
	if !available {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryAgentNotAvailable"
		condition.Message = "The registry agent is not available."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.AgentAvailable = ptr.To(available)
}

func (r *Registry) SetAgentReady(ready bool) {
	condition := metav1.Condition{
		Type:               "AgentReady",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryAgentReady",
		Message:            "The registry agent is ready.",
		ObservedGeneration: r.Generation,
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryAgentNotReady"
		condition.Message = "The registry agent is not ready."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.AgentReady = ptr.To(ready)
}

func (r *Registry) SetMirrorSyncAvailable(available bool) {
	condition := metav1.Condition{
		Type:               "MirrorSyncAvailable",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryMirrorSyncAvailable",
		Message:            "The containerd mirror sync is available.",
		ObservedGeneration: r.Generation,
	}
	if !available {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryMirrorSyncNotAvailable"
		condition.Message = "The containerd mirror sync is not available."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.MirrorSyncAvailable = ptr.To(available)
}

func (r *Registry) SetMirrorSyncReady(ready bool) {
	condition := metav1.Condition{
		Type:               "MirrorSyncReady",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "RegistryMirrorSyncReady",
		Message:            "The containerd mirror sync is ready.",
		ObservedGeneration: r.Generation,
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "RegistryMirrorSyncNotReady"
		condition.Message = "The containerd mirror sync is not ready."
	}
	meta.SetStatusCondition(&r.Status.Conditions, condition)
	r.Status.MirrorSyncReady = ptr.To(ready)
}
