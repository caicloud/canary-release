package util

import (
	"time"

	"github.com/caicloud/clientset/kubernetes/typed/release/v1alpha1"
	releaselister "github.com/caicloud/clientset/listers/release/v1alpha1"
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

type updateCRFunc func(cr *releaseapi.CanaryRelease) error

// UpdateCRWithRetries updates a CR with given applyUpdate function
func UpdateCRWithRetries(crClient v1alpha1.CanaryReleaseInterface, crLister releaselister.CanaryReleaseLister, namespace, name string, applyUpdate updateCRFunc) (*releaseapi.CanaryRelease, error) {
	var cr *releaseapi.CanaryRelease

	retryErr := wait.ExponentialBackoff(DefaultRetry, func() (bool, error) {
		var err error
		cr, err = crLister.CanaryReleases(namespace).Get(name)
		if err != nil {
			return false, err
		}

		cr = cr.DeepCopy()

		// apply the update, them attempte to push it to the apiserver
		if applyErr := applyUpdate(cr); applyErr != nil {
			return false, applyErr
		}

		cr, err = crClient.Update(cr)
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
