package utils

import (
	"context"
	"fmt"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const SERVICE_MONITOR_CRD_NAME = "servicemonitors.monitoring.coreos.com"

// IsCRDEstablished returns whether the CRD is served by the API server
func IsCRDEstablished(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established {
			return condition.Status == apiextensionsv1.ConditionTrue
		}
	}
	return false
}

// IsServiceMonitorCRDEstablished returns whether the ServiceMonitor CRD exists and is established
func IsServiceMonitorCRDEstablished(ctx context.Context, c client.Reader) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, types.NamespacedName{Name: SERVICE_MONITOR_CRD_NAME}, crd); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return IsCRDEstablished(crd), nil
}

// GetNodeSolutionArchiveVersionedName returns the versioned name of the NodeSolutionArchive
func GetNodeSolutionArchiveVersionedName(name string, version string) string {
	return fmt.Sprintf("%s-%s", name, version)
}

// GetSolutionArchiveVersionedName returns the versioned name of the SolutionArchive
func GetSolutionArchiveVersionedName(name string, version string) string {
	return fmt.Sprintf("%s-%s", name, version)
}

// GenerateContainerdHostsToml renders the _default/hosts.toml content from an
// ordered list of mirror host URLs (e.g. "https://10.0.0.1:5000"). Order is
// preserved (the caller orders the list: ClusterIP first, then sorted node IPs).
// An empty list yields an empty string.
func GenerateContainerdHostsToml(mirrorHosts []string) string {
	var b strings.Builder
	for _, host := range mirrorHosts {
		fmt.Fprintf(&b, "[host.\"%s\"]\n", host)
		b.WriteString("  capabilities = [\"pull\", \"resolve\"]\n")
		b.WriteString("  ca = \"ca.crt\"\n")
	}
	return b.String()
}

// GenerateRegistriesConf renders a containers/image registries.conf with one
// [[registry]] block per prefix, rewriting to "<endpoint>/<prefix>".
// Order is preserved. An empty prefix list yields an empty string.
func GenerateRegistriesConf(endpoint string, prefixes []string) string {
	var b strings.Builder
	for _, prefix := range prefixes {
		fmt.Fprintf(&b, "[[registry]]\nprefix = \"%s\"\nlocation = \"%s/%s\"\n", prefix, endpoint, prefix)
	}
	return b.String()
}

// CleanResource cleans the resource by setting the managed fields, resource version, UID, and creation timestamp to empty
func CleanResource(obj client.Object) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
}
