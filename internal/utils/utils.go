package utils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNodeSolutionArchiveVersionedName returns the versioned name of the NodeSolutionArchive
func GetNodeSolutionArchiveVersionedName(name string, version string) string {
	return fmt.Sprintf("%s-%s", name, version)
}

// CleanResource cleans the resource by setting the managed fields, resource version, UID, and creation timestamp to empty
func CleanResource(obj client.Object) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
}
