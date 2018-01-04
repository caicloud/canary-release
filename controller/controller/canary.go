package controller

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/caicloud/canary-release/controller/config"
	"github.com/caicloud/canary-release/pkg/api"
	"github.com/caicloud/canary-release/pkg/util"
	"github.com/caicloud/clientset/informers"
	"github.com/caicloud/clientset/kubernetes"
	releaselisters "github.com/caicloud/clientset/listers/release/v1alpha1"
	apiextensions "github.com/caicloud/clientset/pkg/apis/apiextensions/v1beta1"
	releaseapi "github.com/caicloud/clientset/pkg/apis/release/v1alpha1"
	controllerutil "github.com/caicloud/clientset/util/controller"
	"github.com/caicloud/clientset/util/syncqueue"
	log "github.com/zoumo/logdog"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	corev1 "k8s.io/client-go/listers/core/v1"
	extensionslisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

const (
	proxyNameSuffix = "-proxy"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = releaseapi.SchemeGroupVersion.WithKind(api.CanaryReleaseKind)
)

// CanaryReleaseController ...
type CanaryReleaseController struct {
	proxyImage string

	client kubernetes.Interface
	// crdclient apiextensionsclient.Interface

	factory   informers.SharedInformerFactory
	crLister  releaselisters.CanaryReleaseLister
	rLister   releaselisters.ReleaseLister
	dLister   extensionslisters.DeploymentLister
	podLister corev1.PodLister

	queue *syncqueue.SyncQueue
}

// NewCanaryReleaseController creates a new CanaryReleaseController
func NewCanaryReleaseController(cfg config.Configuration) *CanaryReleaseController {
	factory := informers.NewSharedInformerFactory(cfg.Client, 0)

	crinformer := factory.Release().V1alpha1().CanaryReleases()
	rinformer := factory.Release().V1alpha1().Releases()
	dinformer := factory.Extensions().V1beta1().Deployments()
	podinformer := factory.Core().V1().Pods()

	crc := &CanaryReleaseController{
		proxyImage: cfg.Proxy.Image,
		client:     cfg.Client,
		factory:    factory,
		crLister:   crinformer.Lister(),
		rLister:    rinformer.Lister(),
		dLister:    dinformer.Lister(),
		podLister:  podinformer.Lister(),
	}
	crc.queue = syncqueue.NewPassthroughSyncQueue(&releaseapi.CanaryRelease{}, crc.syncCanaryRelease)
	crinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    crc.addCanaryRelease,
		UpdateFunc: crc.updateCanaryRelease,
		DeleteFunc: crc.deleteCanaryRelease,
	})

	podinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    crc.addPod,
		UpdateFunc: crc.updatePod,
		DeleteFunc: crc.deletePod,
	})

	return crc
}

// Run ...
func (crc *CanaryReleaseController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	log.Info("Startting canary release controller")
	defer log.Info("Shutting down canary release controller")

	if err := crc.ensureResource(); err != nil {
		log.Error("Error ensure canary release", log.Fields{"err": err})
		return
	}

	log.Info("Startting informer factory")
	crc.factory.Start(stopCh)

	log.Info("Wart for all caches synced")
	synced := crc.factory.WaitForCacheSync(stopCh)
	for tpy, sync := range synced {
		if !sync {
			log.Error("Wait for cache sync timeout", log.Fields{"type": tpy})
			return
		}
	}
	log.Info("All cahces have synced, Running Canary Release Controller ...", log.Fields{"worker": workers})

	// start workers
	crc.queue.Run(workers)

	defer func() {
		log.Info("Shutting down controller queue")
		crc.queue.ShutDown()
	}()

	<-stopCh
}

func (crc *CanaryReleaseController) ensureResource() error {
	cr := &apiextensions.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "canaryreleases." + releaseapi.GroupName,
		},
		Spec: apiextensions.CustomResourceDefinitionSpec{
			Group:   releaseapi.GroupName,
			Version: "v1alpha1",
			Scope:   apiextensions.NamespaceScoped,
			Names: apiextensions.CustomResourceDefinitionNames{
				Plural:   "canaryreleases",
				Singular: "canaryrelease",
				Kind:     "CanaryRelease",
				ListKind: "CanaryReleaseList",
			},
		},
	}
	_, err := crc.client.ApiextensionsV1beta1().CustomResourceDefinitions().Create(cr)
	if errors.IsAlreadyExists(err) {
		log.Info("Skip the creation for CustomResourceDefinition CanaryRelease because it has already been created")
		return nil
	}

	if err != nil {
		return err
	}

	log.Info("Create CustomResourceDefinition CanaryRelease successfully")

	return nil
}

func (crc *CanaryReleaseController) selector(cr *releaseapi.CanaryRelease) labels.Set {
	return labels.Set{
		api.LabelKeyCreatedBy: fmt.Sprintf(api.LabelValueFormatCreateby, cr.Namespace, cr.Name),
	}
}

func (crc *CanaryReleaseController) syncCanaryRelease(obj interface{}) error {
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
	ncr, err := crc.crLister.CanaryReleases(cr.Namespace).Get(cr.Name)
	if errors.IsNotFound(err) {
		log.Warn("CanaryRelease has been deleted, clean up", log.Fields{"cr": key})
		return crc.cleanup(cr)
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

	// canary release
	if ncr.DeletionTimestamp != nil {
		return nil
	}

	cr, err = api.CanaryReleaseDeepCopy(ncr)
	if err != nil {
		return err
	}

	// only if the canary release status clearly point out it finished transition
	// then we can cleanup it
	if cr.Status.Phase != releaseapi.CanaryTrasitionNone {
		log.Info("detected a adopted/deprecated Cananry Release, cleanup it", log.Fields{"cr": key})
		return crc.cleanup(cr)
	}

	// find related release
	release, err := crc.rLister.Releases(cr.Namespace).Get(cr.Spec.Release)
	if errors.IsNotFound(err) {
		// if release has been deleted, skip this canary release
		log.Info("Release has been deleted, deprecate this CanaryRelease", log.Fields{"cr": key})
		return crc.deprecate(cr)
	}

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve Release %v from store: %v", key, err))
		return err
	}

	// if release.version != canary.version, skip it
	if release.Status.Version != cr.Spec.Version {
		log.Info("Release.Version != CanaryRelease.Version deprecate this CanaryRelease", log.Fields{"release.version": release.Status.Version, "canary.version": cr.Spec.Version})
		return crc.deprecate(cr)
	}

	ds, err := crc.getDeploymentsForCanaryRelease(cr)
	if err != nil {
		return err
	}

	return crc.sync(cr, ds)
}

func (crc *CanaryReleaseController) deprecate(cr *releaseapi.CanaryRelease) error {
	if cr.Spec.Transition != releaseapi.CanaryTrasitionNone {
		return nil
	}
	patch := fmt.Sprintf(`{"spec": {"transition": "%s"}}`, releaseapi.CanaryTrasitionDeprecated)
	_, err := crc.client.ReleaseV1alpha1().CanaryReleases(cr.Namespace).Patch(cr.Name, types.MergePatchType, []byte(patch))
	return err
}

func (crc *CanaryReleaseController) cleanup(cr *releaseapi.CanaryRelease) error {
	ds, err := crc.getDeploymentsForCanaryRelease(cr)
	if err != nil {
		return err
	}

	policy := metav1.DeletePropagationBackground
	gracePeriodSeconds := int64(60)
	for _, d := range ds {
		err = crc.client.ExtensionsV1beta1().Deployments(d.Namespace).Delete(d.Name, &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSeconds,
			PropagationPolicy:  &policy,
		})
		if err != nil {
			log.Warn("Error clean up canary release", log.Fields{"d.ns": d.Namespace, "d.name": d.Name, "err": err})
			return err
		}
	}

	return nil
}

func (crc *CanaryReleaseController) sync(cr *releaseapi.CanaryRelease, dps []*extensions.Deployment) error {
	desiredDeploy := crc.generateDeployment(cr)

	var err error
	updated := false
	activeDeploy := desiredDeploy

	for _, dp := range dps {
		// two conditions will trigger controller to scale down deployment
		// 1. deployment does not have auto-generated prefix
		// 2. if there are more than one active controllers, there may be many valid deployments.
		//    But we only need one.
		if !strings.HasPrefix(dp.Name, cr.Name+proxyNameSuffix) || updated {
			log.Warn("Delete unexpected proxy", log.Fields{"dp.name": dp.Name, "cr.name": cr.Name})
			crc.client.ExtensionsV1beta1().Deployments(dp.Namespace).Delete(dp.Name, &metav1.DeleteOptions{})
			continue
		}

		updated = true
		activeDeploy = dp
		// no need to update, let proxy do it
	}

	if !updated {
		crc.addCondition(cr, api.NewCondition(api.ReasonCreating, ""))
		log.Info("Create proxy for canary release", log.Fields{"dp.name": desiredDeploy.Name, "cr.name": cr.Name})
		_, err = crc.client.ExtensionsV1beta1().Deployments(cr.Namespace).Create(desiredDeploy)
		if err != nil {
			return err
		}
	}

	return crc.syncStatus(cr, activeDeploy)
}

func (crc *CanaryReleaseController) addCanaryRelease(obj interface{}) {
	cr := obj.(*releaseapi.CanaryRelease)

	if cr.DeletionTimestamp != nil {
		// On a restart of the controller manager, it's possible for an object to
		// show up in a state that is already pending deletion.
		crc.deleteCanaryRelease(cr)
		return
	}

	log.Info("Adding CanaryRelaese", log.Fields{"name": cr.Name})
	crc.queue.Enqueue(cr)

}
func (crc *CanaryReleaseController) updateCanaryRelease(oldObj, curObj interface{}) {
	old := oldObj.(*releaseapi.CanaryRelease)
	cur := curObj.(*releaseapi.CanaryRelease)

	if old.ResourceVersion == cur.ResourceVersion {
		// Periodic resync will send update events for all known Objects.
		// Two different versions of the same Objects will always have different RVs.
		return
	}

	if reflect.DeepEqual(old.Spec, cur.Spec) {
		return
	}

	log.Info("Updating CanaryRelease", log.Fields{"name": old.Name})
	crc.queue.EnqueueAfter(cur, 1*time.Second)

}
func (crc *CanaryReleaseController) deleteCanaryRelease(obj interface{}) {
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

	log.Info("Deleting CanaryRelease", log.Fields{"cr.name": cr.Name, "cr.ns": cr.Namespace})

	crc.queue.Enqueue(cr)
}

func (crc *CanaryReleaseController) getDeploymentsForCanaryRelease(cr *releaseapi.CanaryRelease) ([]*extensions.Deployment, error) {
	// construct selector
	selector := crc.selector(cr).AsSelector()

	// list all
	dList, err := crc.dLister.Deployments(cr.Namespace).List(selector)
	if err != nil {
		return nil, err
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing deployment (see kubernetes#42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		// fresh lb
		fresh, err := crc.client.ReleaseV1alpha1().CanaryReleases(cr.Namespace).Get(cr.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		if fresh.UID != cr.UID {
			return nil, fmt.Errorf("original CanaryRelease %v/%v is gone: got uid %v, wanted %v", cr.Namespace, cr.Name, fresh.UID, cr.UID)
		}
		return fresh, nil
	})
	cm := controllerutil.NewDeploymentControllerRefManager(crc.client, cr, selector, controllerKind, canAdoptFunc)
	result, err := cm.Claim(dList)
	if err == nil {
		// sort deployments
		ret := sortByName(result)
		sort.Sort(ret)
		result = []*extensions.Deployment(ret)
	}
	return result, err
}

func (crc *CanaryReleaseController) generateDeployment(cr *releaseapi.CanaryRelease) *extensions.Deployment {
	terminationGraPeridSeconds := int64(60)
	labels := crc.selector(cr)
	t := true
	replicas := int32(1)
	deploy := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cr.Name + proxyNameSuffix,
			Labels: labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         controllerKind.GroupVersion().String(),
					Kind:               controllerKind.Kind,
					Name:               cr.Name,
					UID:                cr.UID,
					Controller:         &t,
					BlockOwnerDeletion: &t,
				},
			},
		},
		Spec: extensions.DeploymentSpec{
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					TerminationGracePeriodSeconds: &terminationGraPeridSeconds,
					Containers: []v1.Container{
						{
							Name:            "canary-release-proxy",
							Image:           crc.proxyImage,
							ImagePullPolicy: v1.PullAlways,
							Resources:       cr.Spec.Resources,
							Env: []v1.EnvVar{
								{
									Name:  "CANARY_RELEASE_NAME",
									Value: cr.Name,
								},
								{
									Name:  "CANARY_RELEASE_NAMESPACE",
									Value: cr.Namespace,
								},
								{
									Name:  "RELEASE_NAME",
									Value: cr.Spec.Release,
								},
							},
						},
					},
				},
			},
		},
	}

	return deploy
}

// RecheckDeletionTimestamp returns a canAdopt() function to recheck deletion.
//
// The canAdopt() function calls getObject() to fetch the latest value,
// and denies adoption attempts if that object has a non-nil DeletionTimestamp.
func RecheckDeletionTimestamp(getObject func() (metav1.Object, error)) func() error {
	return func() error {
		obj, err := getObject()
		if err != nil {
			return fmt.Errorf("can't recheck DeletionTimestamp: %v", err)
		}
		if obj.GetDeletionTimestamp() != nil {
			return fmt.Errorf("%v/%v has just been deleted at %v", obj.GetNamespace(), obj.GetName(), obj.GetDeletionTimestamp())
		}
		return nil
	}
}

func (crc *CanaryReleaseController) addCondition(cr *releaseapi.CanaryRelease, condition releaseapi.CanaryReleaseCondition) error {
	apply := func(cr *releaseapi.CanaryRelease) error {
		cr.Status.Conditions = append(cr.Status.Conditions, condition)
		return nil
	}
	_, err := util.UpdateCRWithRetries(crc.client.ReleaseV1alpha1().CanaryReleases(cr.Namespace), crc.crLister, cr.Namespace, cr.Name, apply)
	return err
}

func (crc *CanaryReleaseController) addErrorCondition(cr *releaseapi.CanaryRelease, err error) error {
	return crc.addCondition(cr, api.NewConditionFrom(err))
}
