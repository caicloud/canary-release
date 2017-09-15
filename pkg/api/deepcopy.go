package api

import (
	"fmt"

	"github.com/caicloud/clientset/kubernetes/scheme"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	log "github.com/zoumo/logdog"
)

// CanaryReleaseDeepCopy clones the given CanaryRelease and returns a new one.
func CanaryReleaseDeepCopy(cr *releaseapi.CanaryRelease) (*releaseapi.CanaryRelease, error) {
	cri, err := scheme.Scheme.DeepCopy(cr)
	if err != nil {
		log.Error("Unable to deepcopy canary release", log.Fields{"cr.name": cr.Name, "err": err})
		return nil, err
	}

	ncr, ok := cri.(*releaseapi.CanaryRelease)
	if !ok {
		nerr := fmt.Errorf("expected canary release, got %#v", cri)
		log.Error(nerr)
		return nil, err
	}
	return ncr, nil
}
