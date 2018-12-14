package event

import (
	"strings"

	"k8s.io/api/core/v1"
)

// copy from k8s.io/kubernetes/pkg/kubelet/evnets/event.go
const (
	// ContainerUnhealthy describes Unhealthy pod event
	ContainerUnhealthy = "Unhealthy"
	FailedMountVolume  = "FailedMount"
)

// Reasons for pod events
const (

	// FailedCreatePodReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be created.
	FailedCreatePodReason = "FailedCreate"
	// SuccessfulCreatePodReason is added in an event when a pod for a replica set
	// is successfully created.
	SuccessfulCreatePodReason = "SuccessfulCreate"
	// FailedDeletePodReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be deleted.
	FailedDeletePodReason = "FailedDelete"
	// SuccessfulDeletePodReason is added in an event when a pod for a replica set
	// is successfully deleted.
	SuccessfulDeletePodReason = "SuccessfulDelete"
)

// EventByLastTimestamp sorts event by lastTimestamp
type EventByLastTimestamp []*v1.Event

func (x EventByLastTimestamp) Len() int {
	return len(x)
}

func (x EventByLastTimestamp) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (x EventByLastTimestamp) Less(i, j int) bool {
	it := x[i].LastTimestamp
	jt := x[j].LastTimestamp
	return it.After(jt.Time)
}

type EventCase struct {
	EventType string
	Reason    string
	MsgKeys   []string
}

func (c *EventCase) Match(event *v1.Event) bool {
	if event == nil {
		return false
	}
	if c.EventType != "" && c.EventType != event.Type {
		return false
	}

	if c.Reason != "" && c.Reason != event.Reason {
		return false
	}

	if len(c.MsgKeys) == 0 {
		return true
	}

	for _, kw := range c.MsgKeys {
		if strings.Contains(event.Message, kw) {
			// match
			return true
		}
	}
	return false
}
