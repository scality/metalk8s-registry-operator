package utils

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyPatch is a client.Patch implementing server-side apply. It replaces the
// deprecated sigs.k8s.io/controller-runtime/pkg/client.Apply value. The
// non-deprecated alternative on client.Client.Apply() takes a typed
// runtime.ApplyConfiguration, which would require regenerating apply
// configurations for the CRD and Namespace flows we drive here; we keep the
// legacy Patch(ctx, obj, ApplyPatch, ...) call shape and only bypass the
// deprecated helper.
var ApplyPatch client.Patch = applyPatch{}

type applyPatch struct{}

func (applyPatch) Type() types.PatchType { return types.ApplyPatchType }

func (applyPatch) Data(obj client.Object) ([]byte, error) {
	return json.Marshal(obj)
}
