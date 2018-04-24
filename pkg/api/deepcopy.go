package api

import (
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
)

// CanaryReleaseDeepCopy clones the given CanaryRelease and returns a new one.
func CanaryReleaseDeepCopy(cr *releaseapi.CanaryRelease) (*releaseapi.CanaryRelease, error) {
	return cr.DeepCopy(), nil
}
