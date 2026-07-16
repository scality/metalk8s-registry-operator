/*
Copyright 2026 Scality.

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

package helpers

// cluster.go: read-only accessors that describe the shape of the target
// cluster — enumerating worker nodes for scheduling decisions and fetching
// individual Node objects for label/taint patches.

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListWorkerNodes returns the names of all non-control-plane nodes, sorted
// for determinism. In our test environment (5 nodes, node-1 = CP) this
// yields [node-2, node-3, node-4, node-5]; any subset of these may be used
// as registry candidates or as puller nodes.
func ListWorkerNodes(ctx context.Context, c client.Client) ([]string, error) {
	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	var out []string
	for i := range nodes.Items {
		n := &nodes.Items[i]
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		out = append(out, n.Name)
	}
	sort.Strings(out)
	return out, nil
}

// GetNode fetches a single node by name.
func GetNode(ctx context.Context, c client.Client, name string) (*corev1.Node, error) {
	n := &corev1.Node{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, n); err != nil {
		return nil, err
	}
	return n, nil
}

// GetNodeInternalIP returns the first NodeInternalIP from the node's status.
// The e2e cluster (see .github/scripts/prepare-cluster.sh) advertises each
// node on 172.30.100.10x and the runner reaches those IPs directly via
// sshuttle, so we can talk to the RNA's hostPort-exposed :5001 without
// going through kubectl port-forward.
func GetNodeInternalIP(ctx context.Context, c client.Client, name string) (string, error) {
	n, err := GetNode(ctx, c, name)
	if err != nil {
		return "", err
	}
	for _, addr := range n.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("node %s has no InternalIP in status", name)
}
