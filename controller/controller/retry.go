package controller

import (
	"time"

	"github.com/caicloud/canary-release/pkg/api"
	releaseclient "github.com/caicloud/clientset/kubernetes/typed/release/v1alpha1"
	releaselisters "github.com/caicloud/clientset/listers/release/v1alpha1"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

// DefaultRetry is the recommended retry for a conflict where multiple clients
// are making changes to the same resource.
var DefaultRetry = wait.Backoff{
	Steps:    5,
	Duration: 10 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

type updateFunc func(cr *releaseapi.CanaryRelease) error

// UpdateLBWithRetries update loadbalancer with max retries
func updateWithRetries(client releaseclient.CanaryReleaseInterface, crlister releaselisters.CanaryReleaseLister, namespace, name string, applyUpdate updateFunc) (*releaseapi.CanaryRelease, error) {
	var cr *releaseapi.CanaryRelease

	retryErr := wait.ExponentialBackoff(DefaultRetry, func() (bool, error) {
		var err error
		// get from lister
		cr, err = crlister.CanaryReleases(namespace).Get(name)
		if err != nil {
			return false, err
		}
		// deep copy
		obj, deepCopyErr := api.CanaryReleaseDeepCopy(cr)
		if deepCopyErr != nil {
			return false, deepCopyErr
		}

		cr = obj

		// apply change
		if applyErr := applyUpdate(cr); applyErr != nil {
			return false, applyErr
		}

		// update to apiserver
		cr, err = client.Update(cr)
		if err == nil {
			return true, nil
		}
		if errors.IsConflict(err) {
			// retry
			return false, nil
		}
		return false, err
	})

	return cr, retryErr
}
