package api

import (
	"fmt"

	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
)

const (
	CanaryReleaseKind = "CanaryRelease"
)

var (
	LabelKeyCreatedBy        = fmt.Sprintf("%s.%s/created-by", "canary", releaseapi.GroupName)
	LabelValueFormatCreateby = "%s.%s"
)
