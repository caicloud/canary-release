package controller

import (
	"github.com/caicloud/canary-release/pkg/api"
	"github.com/caicloud/canary-release/pkg/util"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
)

func (p *Proxy) addCondition(cr *releaseapi.CanaryRelease, condition releaseapi.CanaryReleaseCondition) error {
	apply := func(cr *releaseapi.CanaryRelease) error {
		cr.Status.Conditions = append(cr.Status.Conditions, condition)
		return nil
	}
	_, err := util.UpdateCRWithRetries(p.cfg.Client.ReleaseV1alpha1().CanaryReleases(p.namespace), p.crLister, cr.Namespace, cr.Name, apply)
	return err
}

func (p *Proxy) addErrorCondition(cr *releaseapi.CanaryRelease, err error) error {
	return p.addCondition(cr, api.NewConditionFrom(err))
}
