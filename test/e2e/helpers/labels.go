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

// labels.go: idempotent label and taint mutations used to drive Registry
// scheduling. Adding/removing RegistryRoleLabel expands/shrinks the
// Registry StatefulSet replica set; NoExecute taints exercise the
// eviction path in the taint-related specs.

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LabelAsRegistry adds the RegistryRoleLabel="" to the given node so the
// operator schedules RNA/RegistryServer StatefulSets on it. Idempotent.
func LabelAsRegistry(ctx context.Context, c client.Client, nodeName string) error {
	return setNodeLabel(ctx, c, nodeName, RegistryRoleLabel, "")
}

// UnlabelAsRegistry removes the RegistryRoleLabel from the node if present.
func UnlabelAsRegistry(ctx context.Context, c client.Client, nodeName string) error {
	return setNodeLabel(ctx, c, nodeName, RegistryRoleLabel, deleteLabelSentinel)
}

// TaintNoExecute applies a NoExecute taint that evicts non-tolerating pods.
// Idempotent.
func TaintNoExecute(ctx context.Context, c client.Client, nodeName, key string) error {
	return setNodeTaint(ctx, c, nodeName, corev1.Taint{
		Key:    key,
		Effect: corev1.TaintEffectNoExecute,
	}, false)
}

// Untaint removes a taint by key/effect from the node.
func Untaint(ctx context.Context, c client.Client, nodeName, key string, effect corev1.TaintEffect) error {
	return setNodeTaint(ctx, c, nodeName, corev1.Taint{
		Key:    key,
		Effect: effect,
	}, true)
}

// ResetRegistryLabels drops the RegistryRoleLabel from every worker node
// so specs can start from a clean baseline.
func ResetRegistryLabels(ctx context.Context, c client.Client) error {
	workers, err := ListWorkerNodes(ctx, c)
	if err != nil {
		return err
	}
	for _, n := range workers {
		if err := UnlabelAsRegistry(ctx, c, n); err != nil {
			return fmt.Errorf("unlabel %s: %w", n, err)
		}
	}
	return nil
}

// deleteLabelSentinel is a marker value: passing it to setNodeLabel means
// "delete the key from the node's labels" rather than "set it to this
// value". Using a real string keeps setNodeLabel a single call-site.
const deleteLabelSentinel = "__DELETE__"

func setNodeLabel(ctx context.Context, c client.Client, nodeName, key, value string) error {
	node, err := GetNode(ctx, c, nodeName)
	if err != nil {
		return err
	}
	if value == deleteLabelSentinel {
		if _, ok := node.Labels[key]; !ok {
			return nil
		}
		patch := client.RawPatch(client.Merge.Type(),
			fmt.Appendf(nil, `{"metadata":{"labels":{%q:null}}}`, key))
		return c.Patch(ctx, node, patch)
	}
	if existing, ok := node.Labels[key]; ok && existing == value {
		return nil
	}
	patch := client.RawPatch(client.Merge.Type(),
		fmt.Appendf(nil, `{"metadata":{"labels":{%q:%q}}}`, key, value))
	return c.Patch(ctx, node, patch)
}

func setNodeTaint(ctx context.Context, c client.Client, nodeName string, taint corev1.Taint, remove bool) error {
	node, err := GetNode(ctx, c, nodeName)
	if err != nil {
		return err
	}
	patched := node.DeepCopy()
	newTaints := patched.Spec.Taints[:0]
	found := false
	for _, t := range patched.Spec.Taints {
		if t.Key == taint.Key && t.Effect == taint.Effect {
			found = true
			continue
		}
		newTaints = append(newTaints, t)
	}
	if !remove {
		newTaints = append(newTaints, taint)
	}
	if remove && !found {
		return nil
	}
	patched.Spec.Taints = newTaints
	return c.Patch(ctx, patched, client.MergeFrom(node))
}
