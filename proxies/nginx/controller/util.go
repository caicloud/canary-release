package controller

import (
	"fmt"
	"syscall"

	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/kubernetes/pkg/util/sysctl"
)

type jsonMap map[string]interface{}

type serviceCollection struct {
	// original service name
	name string
	// original generated service
	// origin service may be modified by canary controller
	// so we need to generate it again
	origin *v1.Service
	// origin service in cluster
	inCluster *v1.Service
	// original generated service copy
	// the only difference between forked service and originGenerated is name
	forked *v1.Service
	// canary release service
	// canary service has different name too
	canary *v1.Service
	// canary service config
	service releaseapi.CanaryService

	// protoPort2upstreamPort contains the k8s service ports to nginx upstream ports map
	// the key is protocol-port
	protoPort2upstreamPort map[string]int32
}

type sortByName []*serviceCollection

func (s sortByName) Len() int {
	return len(s)
}

func (s sortByName) Less(i, j int) bool {
	return s[i].name < s[j].name
}

func (s sortByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type sortByPort []releaseapi.CanaryPort

func (s sortByPort) Len() int {
	return len(s)
}

func (s sortByPort) Less(i, j int) bool {
	return s[i].Port < s[j].Port
}

func (s sortByPort) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// sysctlSomaxconn returns the value of net.core.somaxconn, i.e.
// maximum number of connections that can be queued for acceptance
// http://nginx.org/en/docs/http/ngx_http_core_module.html#listen
func sysctlSomaxconn() int {
	maxConns, err := sysctl.New().GetSysctl("net/core/somaxconn")
	if err != nil || maxConns < 512 {
		glog.V(3).Infof("system net.core.somaxconn=%v (using system default)", maxConns)
		return 511
	}

	return maxConns
}

// sysctlFSFileMax returns the value of fs.file-max, i.e.
// maximum number of open file descriptors
func sysctlFSFileMax() int {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		glog.Errorf("unexpected error reading system maximum number of open file descriptors (RLIMIT_NOFILE): %v", err)
		// returning 0 means don't render the value
		return 0
	}
	return int(rLimit.Max)
}

// protoPortKey generate key for protocol port
func protoPortKey(protocol v1.Protocol, port int32) string {
	return fmt.Sprintf("%s-%d", protocol, port)
}

func getService(objs []runtime.Object, svcName string) (*v1.Service, error) {
	for _, o := range objs {
		svc, ok := o.(*v1.Service)
		if !ok {
			continue
		}
		if svc.Name == svcName {
			return svc, nil
		}
	}

	return nil, fmt.Errorf("Service %v not found", svcName)
}

func renderOwnerReference(cr *releaseapi.CanaryRelease) metav1.OwnerReference {
	t := true
	return metav1.OwnerReference{
		APIVersion:         canaryKind.GroupVersion().String(),
		Kind:               canaryKind.Kind,
		Name:               cr.Name,
		UID:                cr.UID,
		Controller:         &t,
		BlockOwnerDeletion: &t,
	}
}

func renderReleaseOwnerReference(r *releaseapi.Release) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: releaseKind.GroupVersion().String(),
		Kind:       releaseKind.Kind,
		Name:       r.Name,
		UID:        r.UID,
	}
}

func appendOwnerIfNotExists(old []metav1.OwnerReference, pending metav1.OwnerReference) []metav1.OwnerReference {
	found := false
	for _, owner := range old {
		if owner.UID == pending.UID {
			found = true
		}
	}
	if !found {
		old = append(old, pending)
	}
	return old
}

func deleteOwnerIfExists(old []metav1.OwnerReference, pending metav1.OwnerReference) []metav1.OwnerReference {
	var ret []metav1.OwnerReference
	for _, owner := range old {
		if owner.UID == pending.UID {
			continue
		}
		ret = append(ret, owner)
	}
	return ret
}
