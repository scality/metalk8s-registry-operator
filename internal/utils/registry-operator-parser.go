package utils

import (
	"bytes"
	"context"
	"io"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type RegistryComponent struct {
	log                             logr.Logger
	converter                       runtime.UnstructuredConverter
	Namespaces                      []*corev1.Namespace
	CustomResourceDefinitions       []*apiextensionsv1.CustomResourceDefinition
	StatefulSets                    []*appsv1.StatefulSet
	Roles                           []*rbacv1.Role
	ClusterRoles                    []*rbacv1.ClusterRole
	RoleBindings                    []*rbacv1.RoleBinding
	ClusterRoleBindings             []*rbacv1.ClusterRoleBinding
	Certificates                    []*cmv1.Certificate
	ValidatingWebhookConfigurations []*admissionregistrationv1.ValidatingWebhookConfiguration
	UnstructuredObjects             []*unstructured.Unstructured
}

// NewRegistryComponent creates a new RegistryComponent
func NewRegistryComponent(ctx context.Context) *RegistryComponent {
	log := logf.FromContext(ctx)
	return &RegistryComponent{
		log:                             log,
		converter:                       runtime.DefaultUnstructuredConverter,
		Namespaces:                      make([]*corev1.Namespace, 0),
		CustomResourceDefinitions:       make([]*apiextensionsv1.CustomResourceDefinition, 0),
		StatefulSets:                    make([]*appsv1.StatefulSet, 0),
		Roles:                           make([]*rbacv1.Role, 0),
		ClusterRoles:                    make([]*rbacv1.ClusterRole, 0),
		RoleBindings:                    make([]*rbacv1.RoleBinding, 0),
		ClusterRoleBindings:             make([]*rbacv1.ClusterRoleBinding, 0),
		Certificates:                    make([]*cmv1.Certificate, 0),
		ValidatingWebhookConfigurations: make([]*admissionregistrationv1.ValidatingWebhookConfiguration, 0),
		UnstructuredObjects:             make([]*unstructured.Unstructured, 0),
	}
}

// LoadManifests loads the given manifests into the RegistryComponent
func (r *RegistryComponent) LoadManifests(manifests []byte) error {
	decoder := k8syaml.NewYAMLToJSONDecoder(bytes.NewReader(manifests))

	r.log.V(1).Info("--- Decoding and Converting Kubernetes Manifests ---")
	for {
		var obj unstructured.Unstructured

		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			r.log.Error(err, "Error decoding YAML document")
			break
		}
		if len(obj.Object) == 0 {
			continue
		}

		kind := obj.GetKind()
		apiVersion := obj.GetAPIVersion()
		name := obj.GetName()

		r.log.V(1).Info("Found Object", "kind", kind, "apiVersion", apiVersion, "name", name)
		switch kind {
		case "Namespace":
			// The compiler infers T = corev1.Namespace and S = []*corev1.Namespace
			processResource(r, kind, obj, &r.Namespaces)

		case "CustomResourceDefinition":
			// The compiler infers T = apiextensions.CustomResourceDefinition
			processResource(r, kind, obj, &r.CustomResourceDefinitions)

		case "Certificate":
			// The compiler infers T = cmv1.Certificate
			processResource(r, kind, obj, &r.Certificates)

		case "ValidatingWebhookConfiguration":
			// The compiler infers T = admissionregistrationv1.ValidatingWebhookConfiguration
			processResource(r, kind, obj, &r.ValidatingWebhookConfigurations)

		case "StatefulSet":
			// The compiler infers T = appsv1.StatefulSet
			processResource(r, kind, obj, &r.StatefulSets)

		case "Role":
			// The compiler infers T = rbacv1.Role
			processResource(r, kind, obj, &r.Roles)

		case "ClusterRole":
			// The compiler infers T = rbacv1.ClusterRole
			processResource(r, kind, obj, &r.ClusterRoles)

		case "RoleBinding":
			// The compiler infers T = rbacv1.RoleBinding
			processResource(r, kind, obj, &r.RoleBindings)

		case "ClusterRoleBinding":
			// The compiler infers T = rbacv1.ClusterRoleBinding
			processResource(r, kind, obj, &r.ClusterRoleBindings)

		default:
			// The compiler infers T = unstructured.Unstructured and S = []*unstructured.Unstructured
			processResource(r, kind, obj, &r.UnstructuredObjects)
		}
	}
	return nil
}

// T must be a pointer to a struct that implements the runtime.Object interface
// We use a type parameter T that constrains to any type used as a pointer (*Kind)
func processResource[T any, S []*T](
	r *RegistryComponent,
	kind string,
	obj unstructured.Unstructured,
	targetSlice *S,
) {
	var resource T // Declare the resource variable of the generic type T

	// 1. Conversion
	if err := r.converter.FromUnstructured(obj.Object, &resource); err != nil {
		r.log.Error(err, "Error converting", "kind", kind)
		return
	}

	// 2. Logging Success
	r.log.V(1).Info("Successfully converted to struct.", "kind", kind)

	// 3. Appending to the slice (requires dereferencing the slice pointer)
	*targetSlice = append(*targetSlice, &resource)
}

// Flush flushes the RegistryComponent
func (r *RegistryComponent) Flush() {
	r.ValidatingWebhookConfigurations = r.ValidatingWebhookConfigurations[:0]
	r.UnstructuredObjects = r.UnstructuredObjects[:0]
	r.Namespaces = r.Namespaces[:0]
	r.CustomResourceDefinitions = r.CustomResourceDefinitions[:0]
	r.StatefulSets = r.StatefulSets[:0]
	r.Roles = r.Roles[:0]
	r.ClusterRoles = r.ClusterRoles[:0]
	r.RoleBindings = r.RoleBindings[:0]
	r.ClusterRoleBindings = r.ClusterRoleBindings[:0]
	r.Certificates = r.Certificates[:0]
}
