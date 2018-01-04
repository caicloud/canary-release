package controller

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/caicloud/canary-release/pkg/api"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	utilstatus "github.com/caicloud/clientset/util/status"
	log "github.com/zoumo/logdog"

	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// SortPodStatusByName ...
type SortPodStatusByName []releaseapi.PodStatus

func (s SortPodStatusByName) Len() int {
	return len(s)
}

func (s SortPodStatusByName) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s SortPodStatusByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (crc *CanaryReleaseController) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)

	if pod.DeletionTimestamp != nil {
		// On a restart of the controller manager, it's possible for an object to
		// show up in a state that is already pending deletion.
		crc.deletePod(pod)
		return
	}

	cr := crc.getCanaryReleaseForPod(pod)
	if cr == nil {
		return
	}

	crc.queue.Enqueue(cr)
}
func (crc *CanaryReleaseController) updatePod(oldObj, curObj interface{}) {
	old := oldObj.(*v1.Pod)
	cur := curObj.(*v1.Pod)

	if old.ResourceVersion == cur.ResourceVersion {
		// Periodic resync will send update events for all known LoadBalancer.
		// Two different versions of the same LoadBalancer will always have different RVs.
		return
	}

	oldCR := crc.getCanaryReleaseForPod(old)
	curCR := crc.getCanaryReleaseForPod(cur)

	if oldCR != nil {
		if curCR == nil || oldCR.Name != curCR.Name || oldCR.Namespace != curCR.Namespace {
			// CanaryRelease changed
			crc.queue.EnqueueAfter(oldCR, time.Second)
		}
	}

	if curCR != nil {
		crc.queue.EnqueueAfter(curCR, time.Second)
	}
}

func (crc *CanaryReleaseController) deletePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Couldn't get object from tombstone %#v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Tombstone contained object that is not a LoadBalancer %#v", obj))
			return
		}
	}

	cr := crc.getCanaryReleaseForPod(pod)
	if cr == nil {
		return
	}

	crc.queue.Enqueue(cr)
}

func (crc *CanaryReleaseController) getCanaryReleaseForPod(pod *v1.Pod) *releaseapi.CanaryRelease {
	v, ok := pod.Labels[api.LabelKeyCreatedBy]
	if !ok {
		return nil
	}

	namespace, name, err := SplitNamespaceAndNameByDot(v)
	if err != nil {
		log.Error("error get namespace and name", log.Fields{"err": err})
		return nil
	}

	cr, err := crc.crLister.CanaryReleases(namespace).Get(name)
	if errors.IsNotFound(err) {
		// deleted
		return nil
	}
	if err != nil {
		log.Error("can not find loadbalancer for pod", log.Fields{"CR.name": name, "cr.ns": namespace, "pod.name": pod.Name, "err": err})
		return nil
	}
	return cr
}

func (crc *CanaryReleaseController) syncStatus(cr *releaseapi.CanaryRelease, activeDeploy *extensions.Deployment) error {
	proxyStatus := releaseapi.CanaryReleaseProxyStatus{
		Deployment:    activeDeploy.Name,
		Replicas:      *activeDeploy.Spec.Replicas,
		ReadyReplicas: 0,
		TotalReplicas: 0,
		PodStatuses:   []releaseapi.PodStatus{},
	}

	podList, err := crc.podLister.List(crc.selector(cr).AsSelector())
	if err != nil {
		return err
	}

	for _, pod := range podList {
		status := utilstatus.JudgePodStatus(pod)
		podStauts := convertPodStatus(status)
		proxyStatus.TotalReplicas++
		if podStauts.Ready {
			proxyStatus.ReadyReplicas++
		}
		proxyStatus.PodStatuses = append(proxyStatus.PodStatuses, podStauts)
	}

	sort.Sort(SortPodStatusByName(proxyStatus.PodStatuses))
	sort.Sort(SortPodStatusByName(cr.Status.Proxy.PodStatuses))

	if !reflect.DeepEqual(cr.Status.Proxy, proxyStatus) {
		log.Debug("update canary release proxy status", log.Fields{"cr.name": cr.Name, "cr.ns": cr.Namespace})
		_, err := updateWithRetries(
			crc.client.ReleaseV1alpha1().CanaryReleases(cr.Namespace),
			crc.crLister,
			cr.Namespace,
			cr.Name,
			func(cr *releaseapi.CanaryRelease) error {
				cr.Status.Proxy = proxyStatus
				return nil
			},
		)

		if err != nil {
			log.Error("Error update CanaryRelease status", log.Fields{"err": err})
			return err
		}
		return nil
	}

	return nil
}

// SplitNamespaceAndNameByDot returns the namespace and name that
// encoded into the label or value by dot
func SplitNamespaceAndNameByDot(value string) (namespace, name string, err error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected format: %q", value)
	}

	return parts[0], parts[1], nil
}

func convertPodStatus(status utilstatus.PodStatus) releaseapi.PodStatus {
	return releaseapi.PodStatus{
		Name:            status.Name,
		Ready:           status.Ready,
		RestartCount:    status.RestartCount,
		ReadyContainers: status.ReadyContainers,
		TotalContainers: status.TotalContainers,
		NodeName:        status.NodeName,
		Phase:           status.Phase,
		Reason:          status.Reason,
		Message:         status.Message,
	}
}
