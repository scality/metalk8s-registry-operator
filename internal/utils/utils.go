package utils

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// CleanResource cleans the resource by setting the managed fields, resource version, UID, and creation timestamp to empty
func CleanResource(obj client.Object) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
}
