package controller

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/caicloud/canary-release/pkg/api"
	"github.com/caicloud/canary-release/pkg/chart"
	"github.com/caicloud/canary-release/proxies/nginx/config"
	orchestrationlisters "github.com/caicloud/clientset/listers/orchestration/v1alpha1"
	releaselisters "github.com/caicloud/clientset/listers/release/v1alpha1"
	orchestrationapi "github.com/caicloud/clientset/pkg/apis/orchestration/v1alpha1"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	"github.com/caicloud/clientset/util/syncqueue"
	"github.com/caicloud/rudder/pkg/kube"
	"github.com/caicloud/rudder/pkg/render"
	log "github.com/zoumo/logdog"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	forkedServiceSuffix = "-forked"
	canaryServiceSuffix = "-canary"
)

var (
	// canaryKind contains the schema.GroupVersionKind for this controller type.
	canaryKind      = releaseapi.SchemeGroupVersion.WithKind(api.CanaryReleaseKind)
	releaseKind     = releaseapi.SchemeGroupVersion.WithKind("Release")
	applicationKind = orchestrationapi.SchemeGroupVersion.WithKind("Application")
)

// Proxy ...
type Proxy struct {
	cfg           config.Configuration
	release       string
	canaryrelease string
	namespace     string

	crLister    releaselisters.CanaryReleaseLister
	rLister     releaselisters.ReleaseLister
	appLister   orchestrationlisters.ApplicationLister
	svcLister   corelister.ServiceLister
	crInformer  cache.Controller
	rInformer   cache.Controller
	svcInformer cache.Controller
	appInformer cache.Controller

	queue *syncqueue.SyncQueue

	nginx *NginxController
	codec kube.Codec

	runningConfig *config.TemplateConfig
	exiting       bool
	stopCh        chan struct{}
}

// NewProxy ...
func NewProxy(cfg config.Configuration) *Proxy {
	p := &Proxy{
		cfg:           cfg,
		namespace:     cfg.CanaryReleaseNamespace,
		canaryrelease: cfg.CanaryReleaseName,
		release:       cfg.ReleaseName,
		nginx:         NewNginxController(),
		codec:         cfg.Codec,
		stopCh:        make(chan struct{}),
	}

	namespace := cfg.CanaryReleaseNamespace
	var crIndexer, rIndexer, svcIndexer, appIndexer cache.Indexer

	// construct canary release informer
	crIndexer, p.crInformer = cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cfg.Client.ReleaseV1alpha1().CanaryReleases(namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cfg.Client.ReleaseV1alpha1().CanaryReleases(namespace).Watch(options)
			},
		},
		&releaseapi.CanaryRelease{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.addCanaryRelease,
			UpdateFunc: p.updateCanaryRelease,
			DeleteFunc: p.deleteCanaryRelease,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	// construct release informer
	rIndexer, p.rInformer = cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cfg.Client.ReleaseV1alpha1().Releases(namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cfg.Client.ReleaseV1alpha1().Releases(namespace).Watch(options)
			},
		},
		&releaseapi.Release{},
		0,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: p.updateRelease,
			DeleteFunc: p.deleteRelease,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	appIndexer, p.appInformer = cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cfg.Client.OrchestrationV1alpha1().Applications(namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cfg.Client.OrchestrationV1alpha1().Applications(namespace).Watch(options)
			},
		},
		&orchestrationapi.Application{},
		0,
		cache.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	// construct svc informer
	svcIndexer, p.svcInformer = cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cfg.Client.CoreV1().Services(namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cfg.Client.CoreV1().Services(namespace).Watch(options)
			},
		},
		&core.Service{},
		0,
		cache.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	p.queue = syncqueue.NewPassthroughSyncQueue(&releaseapi.CanaryRelease{}, p.syncCanaryRelease)
	p.crLister = releaselisters.NewCanaryReleaseLister(crIndexer)
	p.rLister = releaselisters.NewReleaseLister(rIndexer)
	p.svcLister = corelister.NewServiceLister(svcIndexer)
	p.appLister = orchestrationlisters.NewApplicationLister(appIndexer)

	return p
}

// Run ...
func (p *Proxy) Run(workers int) {
	defer utilruntime.HandleCrash()

	log.Info("Startting canary release controller")
	defer log.Info("Shutting down canary release controller")

	log.Info("Startting informer factory")
	go p.crInformer.Run(p.stopCh)
	go p.rInformer.Run(p.stopCh)
	go p.svcInformer.Run(p.stopCh)
	go p.appInformer.Run(p.stopCh)

	log.Info("Wart for all caches synced")
	if !cache.WaitForCacheSync(p.stopCh,
		p.crInformer.HasSynced,
		p.rInformer.HasSynced,
		p.svcInformer.HasSynced,
		p.appInformer.HasSynced,
	) {
		log.Error("wait for cache sync timeout")
		return
	}

	log.Info("All cahces have synced, Running Canary Release Controller ...", log.Fields{
		"worker":    workers,
		"canary":    p.canaryrelease,
		"namespace": p.namespace,
		"release":   p.release,
	})

	// start workers
	p.queue.Run(workers)

	// start nginx
	go p.nginx.Start()

	<-p.stopCh
}

// Stop shutdowns the proxy
func (p *Proxy) Stop() error {
	// wait proxy to clean up resource
	p.waitForCleanup()
	// close channel to stop informers and queue loop
	close(p.stopCh)
	// stop queue
	p.queue.ShutDown()
	// stop nginx
	p.nginx.Stop()
	return nil
}

// canaryFiltered checks whether needs to filter the canary release
func (p *Proxy) canaryFiltered(cr *releaseapi.CanaryRelease) bool {
	// this cr has been adopted or deprecated
	if cr.Status.Phase != releaseapi.CanaryTrasitionNone {
		return true
	}

	if p.namespace == cr.Namespace && p.canaryrelease == cr.Name {
		return false
	}
	return true
}

// releaseFiltered checks whether needs to filter the release
func (p Proxy) releaseFiltered(r *releaseapi.Release) bool {
	if p.namespace == r.Namespace && p.release == r.Name {
		return false
	}
	return true
}

func (p *Proxy) addCanaryRelease(obj interface{}) {
	cr := obj.(*releaseapi.CanaryRelease)

	if p.canaryFiltered(cr) {
		return
	}

	if cr.DeletionTimestamp != nil {
		// On a restart of the controller manager, it's possible for an object to
		// show up in a state that is already pending deletion.
		p.deleteCanaryRelease(cr)
		return
	}

	log.Info("Adding CanaryRelaese", log.Fields{"name": cr.Name})
	p.queue.Enqueue(cr)

}

func (p *Proxy) updateCanaryRelease(oldObj, curObj interface{}) {
	old := oldObj.(*releaseapi.CanaryRelease)
	cur := curObj.(*releaseapi.CanaryRelease)

	if old.ResourceVersion == cur.ResourceVersion {
		// Periodic resync will send update events for all known Objects.
		// Two different versions of the same Objects will always have different RVs.
		return
	}

	if p.canaryFiltered(cur) {
		return
	}

	if reflect.DeepEqual(old.Spec, cur.Spec) {
		return
	}

	log.Info("Updating CanaryRelease", log.Fields{"name": old.Name})
	p.queue.EnqueueAfter(cur, 1*time.Second)
}

func (p *Proxy) deleteCanaryRelease(obj interface{}) {
	cr, ok := obj.(*releaseapi.CanaryRelease)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Couldn't get object from tombstone %#v", obj))
			return
		}
		cr, ok = tombstone.Obj.(*releaseapi.CanaryRelease)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Tombstone contained object that is not a CanaryRelease %#v", obj))
			return
		}
	}

	if p.canaryFiltered(cr) {
		return
	}

	log.Info("Deleting CanaryRelease", log.Fields{"cr.name": cr.Name, "cr.ns": cr.Namespace})

	p.queue.Enqueue(cr)
}

func (p *Proxy) updateRelease(oldObj, curObj interface{}) {
	old := oldObj.(*releaseapi.Release)
	cur := curObj.(*releaseapi.Release)

	if old.ResourceVersion == cur.ResourceVersion {
		// Periodic resync will send update events for all known Objects.
		// Two different versions of the same Objects will always have different RVs.
		return
	}

	if p.releaseFiltered(cur) {
		return
	}

	if reflect.DeepEqual(old.Spec, cur.Spec) {
		return
	}

	cr, err := p.crLister.CanaryReleases(p.namespace).Get(p.canaryrelease)
	if err != nil {
		return
	}

	log.Info("Updating Release", log.Fields{"name": old.Name})
	p.queue.EnqueueAfter(cr, 1*time.Second)
}

func (p *Proxy) deleteRelease(obj interface{}) {
	r, ok := obj.(*releaseapi.Release)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Couldn't get object from tombstone %#v", obj))
			return
		}
		r, ok = tombstone.Obj.(*releaseapi.Release)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Tombstone contained object that is not a Release %#v", obj))
			return
		}
	}

	if p.releaseFiltered(r) {
		return
	}

	cr, err := p.crLister.CanaryReleases(p.namespace).Get(p.canaryrelease)
	if err != nil {
		return
	}

	log.Info("Deleting Release", log.Fields{"cr.name": cr.Name, "cr.ns": cr.Namespace})

	p.queue.Enqueue(cr)
}

// deprecate changes the canary release's transition to Deprecated
// and defers the processing to next iteration.
func (p *Proxy) deprecate(cr *releaseapi.CanaryRelease) error {

	// if Transition is not None, skip it
	if cr.Spec.Transition != releaseapi.CanaryTrasitionNone {
		return nil
	}

	patch := fmt.Sprintf(`{"spec":{"transition":"%s"}}`, releaseapi.CanaryTrasitionDeprecated)
	_, err := p.cfg.Client.ReleaseV1alpha1().CanaryReleases(cr.Namespace).Patch(cr.Name, types.MergePatchType, []byte(patch))
	return err
}

func (p *Proxy) waitForCleanup() {
	// wait 20 seconds for proxy to cleanup
	wait.PollInfinite(1*time.Second, func() (bool, error) {
		if p.exiting {
			return true, nil
		}
		cr, err := p.crLister.CanaryReleases(p.namespace).Get(p.canaryrelease)
		if errors.IsNotFound(err) {
			// canary release has been deleted, need to wait
			return true, nil
		}
		if err != nil {
			// need retry
			return false, nil
		}

		if cr.Spec.Transition == releaseapi.CanaryTrasitionNone {
			// the transition is none, but want to stop.
			// That means you don't need to wait for clean up
			return true, nil
		}

		return false, nil
	})

}

// syncCanaryRelease will sync the canary release with the given obj.
// This function is not meant to be invoked concurrently with the same obj.
func (p *Proxy) syncCanaryRelease(obj interface{}) error {

	// type assertion
	cr, ok := obj.(*releaseapi.CanaryRelease)
	if !ok {
		return fmt.Errorf("expect canary release, got:%v", obj)
	}

	// get obj key
	key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(cr)

	// record time
	startTime := time.Now()
	defer func() {
		log.Debug("Finished syncing canary release", log.Fields{"cr": key, "usedTime": time.Since(startTime)})
	}()

	// find obj in local store
	ncr, err := p.crLister.CanaryReleases(cr.Namespace).Get(cr.Name)
	if errors.IsNotFound(err) {
		log.Warn("CanaryRelease has been deleted, clean up", log.Fields{"cr": key})
		return p.cleanup(cr)
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve CanaryRelease %v from store: %v", key, err))
		return err
	}

	// fresh cr
	if cr.UID != ncr.UID {
		// original canary release is gone
		return nil
	}

	if ncr.DeletionTimestamp != nil {
		return nil
	}

	// deep copy
	cr = ncr.DeepCopy()

	if cr.Spec.Transition != releaseapi.CanaryTrasitionNone {
		log.Info("detected a adopted/deprecated Cananry Release, cleanup it", log.Fields{"cr": key})
		return p.cleanup(cr)
	}

	// find related release
	release, err := p.rLister.Releases(cr.Namespace).Get(cr.Spec.Release)
	if errors.IsNotFound(err) {
		// if release has been deleted, deprecate this canary release
		log.Info("Release has been deleted, deprecate this CanaryRelease", log.Fields{"cr": key})
		return p.deprecate(cr)
	}

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve Release %v from store: %v", key, err))
		return err
	}

	// if release.version != canary.version, deprecate it
	if release.Status.Version != cr.Spec.Version {
		log.Info("Release.Version != CanaryRelease.Version deprecate this CanaryRelease", log.Fields{"release.version": release.Status.Version, "canary.version": cr.Spec.Version})
		return p.deprecate(cr)
	}

	return p.sync(cr, release)
}

func (p *Proxy) sync(cr *releaseapi.CanaryRelease, release *releaseapi.Release) error {
	p.addCondition(cr, api.NewCondition(api.ReasonUpdating, ""))
	err := p._sync(cr, release)
	if err != nil {
		p.addCondition(cr, api.NewConditionFrom(err))
	} else {
		p.addCondition(cr, api.NewCondition(api.ReasonAvailable, ""))
	}
	return err
}

func (p *Proxy) _sync(cr *releaseapi.CanaryRelease, release *releaseapi.Release) error {
	// generate origin resources from release's template and config
	// generate canary resources from release's template and canary's config, with owner reference
	// find origin service
	// fork origin service and change it's name, add owner reference
	// find canay release service and change it's name
	// update nginx config
	// find original service change it's target port and selector

	canaryOwner := renderOwnerReference(cr)
	releaseOwner := renderReleaseOwnerReference(release)

	// Step 1
	originObj, err := p.renderRelease(release, cr)
	if err != nil {
		log.Errorf("Error render release objects, err: %v", err)
		return err
	}
	// Step 2
	canaryObj, err := p.renderCanaryRelease(release, cr)
	if err != nil {
		log.Errorf("Error render canary objects, err: %v", err)
		return err
	}
	// Step 3
	svcCol, err := p.renderService(originObj, canaryObj, cr)
	if err != nil {
		log.Errorf("Error render service collections %v", err)
		return err
	}

	// update or create canary resources  add canary owner and release owner
	lastManifest := render.SplitManifest(cr.Status.Manifest)
	// service in canary objects have been changed
	manifest, _ := p.codec.ObjectsToResources(canaryObj)
	if reflect.DeepEqual(lastManifest, manifest) {
		log.Info("manifest is not changed, skip this sync")
		return nil
	}
	err = p.cfg.ReleaseClient.Update(cr.Namespace, lastManifest, manifest, kube.UpdateOptions{
		OwnerReferences: []metav1.OwnerReference{
			canaryOwner,
			releaseOwner,
		},
	})
	if err != nil {
		log.Errorf("Error update manifest, err: %v", err)
		return err
	}

	// update status - patch manifest
	patch, _ := json.Marshal(jsonMap{
		"status": jsonMap{
			"manifest": render.MergeResources(manifest),
		},
	})
	_, err = p.cfg.Client.ReleaseV1alpha1().CanaryReleases(p.namespace).Patch(p.canaryrelease, types.MergePatchType, patch)

	if err != nil {
		log.Errorf("Error update canary release status.manifest, %v", err)
		return err
	}

	// Step 4
	// get tcp and udp upsteam
	nginxConfig := config.NewDefaultTemplateConfig()
	nginxConfig.TCPBackends, nginxConfig.UDPBackends = p.getUpsteamService(svcCol)

	// check if need to update
	if p.runningConfig != nil && p.runningConfig.Equal(&nginxConfig) {
		log.Info("template config is not changed")
		return nil
	}

	// Step 5
	// try to create forked service
	for _, svccol := range svcCol {
		// add owner reference to service
		svccol.forked.OwnerReferences = appendOwnerIfNotExists(svccol.forked.OwnerReferences, canaryOwner)
		_, err := p.cfg.Client.CoreV1().Services(p.namespace).Create(svccol.forked)
		if errors.IsAlreadyExists(err) {
			continue
		}
		if err != nil {
			err = fmt.Errorf("Error create forked service, err: %v", err)
			return err
		}
	}

	// Step 6
	// update nginx config
	err = p.nginx.OnUpdate(nginxConfig)
	if err != nil {
		err = fmt.Errorf("Error reload nginx, err: %v", err)
		log.Error(err)
		return err
	}

	// Step 6
	// update original service
	for _, col := range svcCol {
		generated := col.origin
		inCluster := col.inCluster

		// change service selector
		generated.Spec.Selector = map[string]string{
			api.LabelKeyCreatedBy: fmt.Sprintf(api.LabelValueFormatCreateby, p.namespace, p.canaryrelease),
		}

		// change target port

		for i, port := range generated.Spec.Ports {
			port.TargetPort = intstr.FromInt(int(col.protoPort2upstreamPort[protoPortKey(port.Protocol, port.Port)]))
			generated.Spec.Ports[i] = port
		}

		if reflect.DeepEqual(generated.Spec.Ports, inCluster.Spec.Ports) &&
			reflect.DeepEqual(generated.Spec.Selector, inCluster.Spec.Selector) {
			continue
		}

		// add in cluster owner references (release controller) and generated owner referecens (canary release controller)
		inCluster.OwnerReferences = appendOwnerIfNotExists(inCluster.OwnerReferences, canaryOwner)
		patchTargetPort(inCluster.Spec.Ports, generated.Spec.Ports)
		inCluster.Spec.Selector = generated.Spec.Selector

		_, err = p.cfg.Client.CoreV1().Services(p.namespace).Update(inCluster)
		if err != nil {
			log.Errorf("Error update original service %v, err: %v", generated.Name, err)
			return err
		}

	}
	// set running config
	p.runningConfig = &nginxConfig
	return nil
}

func (p *Proxy) getUpsteamService(svcCol []*serviceCollection) ([]api.L4Service, []api.L4Service) {

	getWeight := func(weight *int32) (int32, int32) {
		if weight == nil {
			return 0, 100
		}
		canaryWeight := *weight
		if *weight >= 100 {
			canaryWeight = 100
		}
		if *weight <= 0 {
			canaryWeight = 0
		}
		return canaryWeight, 100 - canaryWeight
	}

	upsteamPort := int32(8080)

	// sort servce by name
	cols := sortByName(svcCol)
	sort.Sort(cols)

	var tcpService, udpService []api.L4Service

	for _, col := range cols {
		// sort ports
		ports := sortByPort(col.service.Ports)
		sort.Sort(ports)

		if col.protoPort2upstreamPort == nil {
			col.protoPort2upstreamPort = make(map[string]int32)
		}

		for _, port := range ports {
			// HTTP HTTPS TCP are considered as TCP protocol
			protocol := core.ProtocolTCP
			if port.Protocol == releaseapi.ProtocolUDP {
				protocol = core.ProtocolUDP
			}

			canaryWeight, originWeight := getWeight(port.Config.Weight)

			upsteam := api.L4Service{
				Port: upsteamPort,
				Backend: api.L4Backend{
					Port:      port.Port,
					Name:      col.name,
					Namespace: p.namespace,
					Protocol:  protocol,
				},
				Endpoints: []api.Endpoint{
					{
						Address: col.forked.Name,
						Port:    port.Port,
						Weight:  originWeight,
					},
					{
						Address: col.canary.Name,
						Port:    port.Port,
						Weight:  canaryWeight,
					},
				},
			}

			if protocol == core.ProtocolTCP {
				tcpService = append(tcpService, upsteam)
			} else {
				udpService = append(udpService, upsteam)
			}

			col.protoPort2upstreamPort[protoPortKey(protocol, port.Port)] = upsteamPort

			upsteamPort++
		}
	}

	return tcpService, udpService
}

// render all needed services
// all generated services' type is ClusterIP
func (p *Proxy) renderService(originObj, canaryObj []runtime.Object, cr *releaseapi.CanaryRelease) ([]*serviceCollection, error) {
	services := make([]*serviceCollection, 0, len(cr.Spec.Service))
	for _, svc := range cr.Spec.Service {
		var err error
		// add canary service config
		s := &serviceCollection{
			service: svc,
		}

		// get origin generated object
		s.origin, err = getService(originObj, svc.Service)
		if err != nil {
			log.Errorf("Error get service %v from origin objects, err: %v", svc.Service, err)
			return nil, err
		}

		// fork origin service
		// change it's name and add owner reference
		copy := *s.origin
		// change forked service name
		copy.Name += forkedServiceSuffix
		// reset nodePort, avoid conflict
		if copy.Spec.Type == core.ServiceTypeNodePort {
			for i := range copy.Spec.Ports {
				copy.Spec.Ports[i].NodePort = 0
			}
		}
		// change forked service type to ClusterIP
		copy.Spec.Type = core.ServiceTypeClusterIP
		s.forked = &copy

		// get in-cluster service
		s.inCluster, err = p.svcLister.Services(p.namespace).Get(svc.Service)
		if err != nil {
			log.Errorf("Error get service %v from lister, err: %v", svc.Service, err)
			return nil, err
		}

		// get canary service
		s.canary, err = getService(canaryObj, svc.Service)
		if err != nil {
			log.Errorf("Error get service %v from canary objects, err: %v", svc.Service, err)
			return nil, err
		}
		// change canary service name
		s.canary.Name += canaryServiceSuffix
		// reset nodePort, avoid conflict
		if s.canary.Spec.Type == core.ServiceTypeNodePort {
			for i := range s.canary.Spec.Ports {
				s.canary.Spec.Ports[i].NodePort = 0
			}
		}
		// change canary service type to ClusterIP
		s.canary.Spec.Type = core.ServiceTypeClusterIP
		services = append(services, s)
	}

	return services, nil
}

func (p *Proxy) renderRelease(release *releaseapi.Release, cr *releaseapi.CanaryRelease) ([]runtime.Object, error) {

	carry, err := render.CarrierForManifest(release.Status.Manifest)
	if err != nil {
		return nil, err
	}
	// get canary release resources
	ress, err := carry.ResourcesOf(cr.Spec.Path)
	if err != nil {
		return nil, fmt.Errorf("Error find resources from carrier with path %v", err)
	}
	// render to objects
	objs, err := p.codec.ResourcesToObjects(ress)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

// render objects from release's template and canary config
func (p *Proxy) renderCanaryRelease(release *releaseapi.Release, cr *releaseapi.CanaryRelease) ([]runtime.Object, error) {
	canaryConfig, err := chart.ReplaceConfig(release.Spec.Config, cr.Spec.Path, cr.Spec.Config)
	if err != nil {
		return nil, err
	}

	r := render.NewRender()
	carry, err := r.Render(&render.RenderOptions{
		Namespace: release.Namespace,
		Release:   release.Name,
		Version:   release.Status.Version,
		Template:  release.Spec.Template,
		Config:    canaryConfig,
	})
	if err != nil {
		return nil, err
	}

	// get canary release resources
	// the path to resource has been changed
	ress, err := carry.ResourcesOf(cr.Spec.Path)
	if err != nil {
		return nil, fmt.Errorf("Error find resources from carrier with path %v", err)
	}

	// render to objects
	objs, err := p.codec.ResourcesToObjects(ress)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

func (p *Proxy) cleanup(cr *releaseapi.CanaryRelease) error {
	err := p._cleanup(cr)
	if err != nil {
		p.addErrorCondition(cr, err)
	} else {
		p.addCondition(cr, api.NewCondition(string(cr.Spec.Transition), ""))
	}
	return err
}

func (p *Proxy) _cleanup(cr *releaseapi.CanaryRelease) error {
	if cr.Status.Phase != releaseapi.CanaryTrasitionNone {
		// transition finished
		return nil
	}

	transition := cr.Spec.Transition
	_, err := p.crLister.CanaryReleases(cr.Namespace).Get(cr.Name)
	if errors.IsNotFound(err) && transition != releaseapi.CanaryTrasitionAdopted {
		transition = releaseapi.CanaryTrasitionDeprecated
	}

	canaryOwner := renderOwnerReference(cr)
	rClient := p.cfg.Client.ReleaseV1alpha1().Releases(cr.Namespace)
	crClient := p.cfg.Client.ReleaseV1alpha1().CanaryReleases(cr.Namespace)
	appClient := p.cfg.Client.OrchestrationV1alpha1().Applications(cr.Namespace)
	svcClient := p.cfg.Client.CoreV1().Services(cr.Namespace)

	// get service from lister with suffix
	getSvcs := func(suffix string) ([]*core.Service, error) {
		var svcs []*core.Service
		for _, svc := range cr.Spec.Service {
			name := svc.Service + suffix
			original, err := p.svcLister.Services(p.namespace).Get(name)
			if errors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			svcs = append(svcs, original)
		}
		return svcs, nil
	}

	// delete services
	deleteSvcs := func(svcs []*core.Service) {
		background := metav1.DeletePropagationBackground
		for _, svc := range svcs {
			err := svcClient.Delete(svc.Name, &metav1.DeleteOptions{
				PropagationPolicy: &background,
			})
			if errors.IsNotFound(err) {
				log.Warnf("delete service %v not found", svc.Name)
			}
		}
	}

	// revocer service relate origin and target services whith name suffix
	// use all fields in target service but original name and owner references
	// to recover the origin service
	recoverSvcs := func(origin, target []*core.Service, suffix string, deleteOwner bool) error {
		for _, o := range origin {

			var fs *core.Service
			for _, f := range target {
				if f.Name == o.Name+suffix {
					fs = f
					break
				}
			}

			if fs == nil {
				continue
			}
			// deep copy target service
			o = o.DeepCopy()
			if deleteOwner {
				o.OwnerReferences = deleteOwnerIfExists(o.OwnerReferences, canaryOwner)
			}
			patchTargetPort(o.Spec.Ports, fs.Spec.Ports)
			o.Spec.Selector = fs.Spec.Selector
			_, err := svcClient.Update(o)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if transition == releaseapi.CanaryTrasitionAdopted {
		// apply change to release
		// find related release
		release, err := p.rLister.Releases(cr.Namespace).Get(cr.Spec.Release)
		if errors.IsNotFound(err) {
			// if release has been deleted, deprecate this canary release
			log.Info("Release has been deleted, deprecate this CanaryRelease", log.Fields{"cr": cr.Name})
			return p.deprecate(cr)
		}

		if err != nil {
			utilruntime.HandleError(fmt.Errorf("Unable to retrieve Release %v from store: %v", cr.Spec.Release, err))
			return err
		}

		if cr.Spec.Version != release.Status.Version {
			return p.deprecate(cr)
		}

		// use canary service cover original
		// origin service has two owner now
		originalService, canaryService, forkedService, err := getRelatedAndRecoverSvcs(canaryServiceSuffix, false, getAndrecoverSvcFunc{getSvcs, recoverSvcs})
		if err != nil {
			return err
		}

		// generate new config
		canaryConfig, err := chart.ReplaceConfig(release.Spec.Config, cr.Spec.Path, cr.Spec.Config)
		if err != nil {
			return err
		}
		controller := getControllerOf(release)
		if controller == nil {
			// update release
			release.Spec.Config = canaryConfig
			_, err = rClient.Update(release)
			if err != nil {
				return err
			}
		} else {
			// update release config in application
			app, err := p.appLister.Applications(cr.Namespace).Get(controller.Name)
			if err != nil {
				return err
			}

			app = app.DeepCopy()
			for i, v := range app.Spec.Graph.Vertexes {
				if v.Name == release.Name {
					app.Spec.Graph.Vertexes[i].Spec.Config = canaryConfig
					break
				}
			}
			_, err = appClient.Update(app)
			if err != nil {
				return err
			}
		}

		// wait for release updated
		wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) {
			r, err := p.cfg.Client.ReleaseV1alpha1().Releases(release.Namespace).Get(release.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if r.Status.Version > release.Status.Version {
				return true, nil
			}
			return false, nil
		})

		// change owner reference to Release for all manifest
		owner := renderReleaseOwnerReference(release)
		ownerPatch := []byte(fmt.Sprintf(`{"metadata":{"ownerReferences":[{"apiVersion":"%s","kind":"%s","name":"%s","uid":"%s"}]}}`, owner.APIVersion, owner.Kind, owner.Name, owner.UID))
		objs, accessors, err := p.codec.AccessorsForResources(render.SplitManifest(cr.Status.Manifest))
		for i, obj := range objs {
			accessor := accessors[i]
			gvk := obj.GetObjectKind().GroupVersionKind()
			if gvk.Kind == "Service" {
				// skip service to avoid unexpected deletion
				// if you set the owner to release, release controller will
				// take over the resources, it find that the resources don't
				// match the spec, so controller delete the redundant resource
				// we expect to delete these services manually.
				continue
			}
			client, _ := p.cfg.ReleaseClientPool.ClientFor(gvk, p.namespace)
			_, err := client.Patch(accessor.GetName(), types.MergePatchType, ownerPatch)
			if err != nil {
				return err
			}
		}

		// reset owner
		for _, svc := range originalService {
			// reset the owner reference to let release controller take over these services
			svcClient.Patch(svc.Name, types.MergePatchType, ownerPatch)
		}

		// delete forkedServices
		deleteSvcs(forkedService)
		// delete canaryServices
		deleteSvcs(canaryService)

	} else if transition == releaseapi.CanaryTrasitionDeprecated {

		// maybe release has been deleted, the originalService will be empty
		// use forked service cover original
		_, _, forkedService, err := getRelatedAndRecoverSvcs(forkedServiceSuffix, true, getAndrecoverSvcFunc{getSvcs, recoverSvcs})

		// delete manifest
		err = p.cfg.ReleaseClient.Delete(p.namespace, render.SplitManifest(cr.Status.Manifest), kube.DeleteOptions{})
		if err != nil {
			log.Errorf("Error delete manifest from canary release, %v", err)
		}

		// delete forked service
		deleteSvcs(forkedService)
	}

	patch := fmt.Sprintf(`{"status":{"manifest":null,"phase":"%s"}}`, cr.Spec.Transition)
	crClient.Patch(cr.Name, types.MergePatchType, []byte(patch))
	p.runningConfig = nil
	p.exiting = true
	return nil
}

func patchTargetPort(source, patch []core.ServicePort) {
	for i, p := range source {
		for _, pp := range patch {
			if isSamePort(p, pp) {
				source[i].TargetPort = pp.TargetPort
				break
			}
		}
	}
}

func isSamePort(s, p core.ServicePort) bool {
	if len(s.Name) != 0 && len(p.Name) != 0 && s.Name == p.Name {
		return true
	} else if s.Protocol == p.Protocol && s.Port == p.Port {
		return true
	}
	return false
}

func getControllerOf(release *releaseapi.Release) *metav1.OwnerReference {
	for i := range release.OwnerReferences {
		owner := &release.OwnerReferences[i]
		if owner.APIVersion == applicationKind.GroupVersion().String() &&
			owner.Kind == applicationKind.Kind {
			return owner
		}
	}
	return nil
}
