package utils

import "fmt"

// GetNodeSolutionArchiveVersionedName returns the versioned name of the NodeSolutionArchive
func GetNodeSolutionArchiveVersionedName(name string, version string) string {
	return fmt.Sprintf("%s-%s", name, version)
}
