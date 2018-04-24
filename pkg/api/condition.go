package api

import (
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ReasonAvailable  = "Available"
	ReasonCreating   = "Creating"
	ReasonUpdating   = "Updating"
	ReasonDeprecated = string(releaseapi.CanaryTrasitionDeprecated)
	ReasonAdopted    = string(releaseapi.CanaryTrasitionAdopted)
	ReasonError      = "Error"
)

// NewConditionFrom creates a new condition from error
func NewConditionFrom(err error) releaseapi.CanaryReleaseCondition {
	return NewCondition(ReasonError, err.Error())
}

// NewCondition creates a new Condition
func NewCondition(reason, message string) releaseapi.CanaryReleaseCondition {
	var typ releaseapi.CanaryReleaseConditionType
	switch reason {
	case ReasonAvailable:
		typ = releaseapi.CanaryReleaseAvailable
	case ReasonDeprecated, ReasonAdopted:
		typ = releaseapi.CanaryReleaseArchived
	case ReasonCreating, ReasonUpdating:
		typ = releaseapi.CanaryReleaseProgressing
	case ReasonError:
		typ = releaseapi.CanaryReleaseFailure
	}

	condition := releaseapi.CanaryReleaseCondition{
		Type:               typ,
		Status:             core.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	return condition
}
