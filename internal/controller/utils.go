package controller

import (
	"fmt"
	"hash/fnv"
)

// getHash32Name returns a 32-bit hash of the input string - hexadecimal representation
func getHash32Name(input string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(input)) //nolint:errcheck // Write() never returns an error for the FNV implementation
	hashSum := hasher.Sum32()
	return fmt.Sprintf("%x", hashSum)
}
