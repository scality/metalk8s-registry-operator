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

// pull.go: ephemeral puller Pod scheduler and verifier. Creates a Pod
// pinned to a chosen worker with a chosen image reference and waits for
// Kubernetes to report either a populated container ImageID (pull
// succeeded — mirror routed the request through the operator's registry)
// or an ImagePullBackOff / ErrImagePull waiting reason (pull failed).

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PullResult captures whether containerd successfully pulled the image
// requested by a puller Pod. The e2e tests never require the container to
// actually run — a puller image is just some random bytes wrapped in a valid
// OCI layer — so we key off Kubernetes' own signal: once a container status
// has a non-empty `imageID` the image is present locally, which necessarily
// means the pull succeeded.
type PullResult struct {
	// Success is true iff the pod reached a state where the image was
	// pulled (imageID is populated on the container status).
	Success bool
	// Reason and Message describe the *last* container-status waiting state
	// observed while polling; they are the informative bits to log on
	// failure (e.g. `ImagePullBackOff`, `back-off pulling image ...`).
	Reason  string
	Message string
	// PodName is the ephemeral pod name Kubernetes assigned; useful for
	// `kubectl logs` follow-ups on failures.
	PodName string
}

// PullerPodSpec describes a puller job. Namespace is created if missing.
type PullerPodSpec struct {
	Name      string
	Namespace string
	NodeName  string
	Image     string
}

// StartPullerPod creates a Pod that immediately requests `Image`, pinned to
// `NodeName` with imagePullPolicy: Always so the mirror is exercised on
// every invocation. The Pod's command intentionally exits with a runtime
// error (there is no /manager in our synthetic images) — that's fine, we
// only look at the pull status.
func StartPullerPod(ctx context.Context, c client.Client, spec PullerPodSpec) (*corev1.Pod, error) {
	if spec.Namespace == "" {
		spec.Namespace = "default"
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "metalk8s-registry-operator-e2e",
				"e2e.metalk8s.scality.com/role": "puller",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			// Pin the pod to the target node so we know which node's
			// containerd is exercising the mirror config.
			NodeName: spec.NodeName,
			// A very short activeDeadlineSeconds bounds the pod's total
			// lifetime should our runtime probe never notice it entered a
			// terminal state.
			ActiveDeadlineSeconds: ptr.To[int64](300),
			Containers: []corev1.Container{
				{
					Name:            "puller",
					Image:           spec.Image,
					ImagePullPolicy: corev1.PullAlways,
				},
			},
			// Tolerate every taint so puller pods are not evicted before we
			// can observe their pull status (tainting a registry node must
			// not evict the puller too).
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
		},
	}

	if err := c.Create(ctx, pod); err != nil {
		return nil, fmt.Errorf("create puller pod: %w", err)
	}
	return pod, nil
}

// PullStatus inspects a puller pod's containerStatuses and returns whether
// the image was fetched (Success=true) or is stuck in an image-pull error
// (Success=false). It also propagates the last observed waiting reason on
// failure, and works both for a Running/Terminated container (successful
// pull) and for a still-Waiting container (retryable state).
func PullStatus(ctx context.Context, c client.Client, name, namespace string) (PullResult, error) {
	pod := &corev1.Pod{}
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, pod); err != nil {
		return PullResult{}, err
	}
	res := PullResult{PodName: pod.Name}
	if len(pod.Status.ContainerStatuses) == 0 {
		res.Reason = "PendingScheduling"
		res.Message = string(pod.Status.Phase)
		return res, nil
	}
	cs := pod.Status.ContainerStatuses[0]
	if cs.ImageID != "" {
		res.Success = true
		return res, nil
	}
	if cs.State.Waiting != nil {
		res.Reason = cs.State.Waiting.Reason
		res.Message = cs.State.Waiting.Message
		return res, nil
	}
	res.Reason = "Unknown"
	res.Message = fmt.Sprintf("phase=%s", pod.Status.Phase)
	return res, nil
}

// IsImagePullErrorReason reports whether the given container-status waiting
// reason indicates that containerd failed the pull.
func IsImagePullErrorReason(reason string) bool {
	switch reason {
	case "ErrImagePull",
		"ImagePullBackOff",
		"RegistryUnavailable",
		"InvalidImageName",
		"ImageInspectError":
		return true
	}
	return strings.Contains(reason, "ImagePull")
}

// DeletePullerPod removes a puller pod immediately (grace 0).
func DeletePullerPod(ctx context.Context, c client.Client, name, namespace string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := c.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
