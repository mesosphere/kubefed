/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	restclient "k8s.io/client-go/rest"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/kubefed/pkg/apis/core/typeconfig"
	fedv1b1 "sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
	genericclient "sigs.k8s.io/kubefed/pkg/client/generic"
	"sigs.k8s.io/kubefed/pkg/controller/util"
	"sigs.k8s.io/kubefed/pkg/metrics"
)

const (
	allClustersKey = "ALL_CLUSTERS"
)

// KubeFedStatusController collects the status of resources in member
// clusters.
type KubeFedStatusController struct {
	// For triggering reconciliation of all target resources. This is
	// used when a new cluster becomes available.
	clusterDeliverer *util.DelayingDeliverer

	// Informer for resources in member clusters
	informer util.FederatedInformer

	// Store for the federated type
	federatedStore cache.Store
	// Informer for the federated type
	federatedController cache.Controller

	// Store for the status of the federated type
	statusStore cache.Store
	// Informer for the status of the federated type
	statusController cache.Controller

	worker util.ReconcileWorker

	clusterAvailableDelay   time.Duration
	clusterUnavailableDelay time.Duration
	smallDelay              time.Duration

	cacheSyncTimeout time.Duration

	typeConfig typeconfig.Interface

	client       genericclient.Client
	statusClient util.ResourceClient

	fedNamespace string
}

// StartKubeFedStatusController starts a new status controller for a type config
func StartKubeFedStatusController(controllerConfig *util.ControllerConfig, stopChan <-chan struct{}, typeConfig typeconfig.Interface) error {
	controller, err := newKubeFedStatusController(controllerConfig, typeConfig)
	if err != nil {
		return err
	}
	if controllerConfig.MinimizeLatency {
		controller.minimizeLatency()
	}
	klog.Infof("Starting status controller for %q", typeConfig.GetFederatedType().Kind)
	controller.Run(stopChan)
	return nil
}

// newKubeFedStatusController returns a new status controller for the federated type
func newKubeFedStatusController(controllerConfig *util.ControllerConfig, typeConfig typeconfig.Interface) (*KubeFedStatusController, error) {
	federatedAPIResource := typeConfig.GetFederatedType()
	statusAPIResource := typeConfig.GetStatusType()
	if statusAPIResource == nil {
		return nil, errors.Errorf("Status collection is not supported for %q", federatedAPIResource.Kind)
	}
	userAgent := fmt.Sprintf("%s-controller", strings.ToLower(statusAPIResource.Kind))
	kubeConfig := restclient.CopyConfig(controllerConfig.KubeConfig)
	restclient.AddUserAgent(kubeConfig, userAgent)
	client := genericclient.NewForConfigOrDie(kubeConfig)

	federatedTypeClient, err := util.NewResourceClient(kubeConfig, &federatedAPIResource)
	if err != nil {
		return nil, err
	}

	statusClient, err := util.NewResourceClient(kubeConfig, statusAPIResource)
	if err != nil {
		return nil, err
	}

	s := &KubeFedStatusController{
		clusterAvailableDelay:   controllerConfig.ClusterAvailableDelay,
		clusterUnavailableDelay: controllerConfig.ClusterUnavailableDelay,
		smallDelay:              time.Second * 3,
		cacheSyncTimeout:        controllerConfig.CacheSyncTimeout,
		typeConfig:              typeConfig,
		client:                  client,
		statusClient:            statusClient,
		fedNamespace:            controllerConfig.KubeFedNamespace,
	}

	s.worker = util.NewReconcileWorker(strings.ToLower(statusAPIResource.Kind), s.reconcile, util.WorkerOptions{
		WorkerTiming: util.WorkerTiming{
			ClusterSyncDelay: s.clusterAvailableDelay,
		},
		MaxConcurrentReconciles: int(controllerConfig.MaxConcurrentStatusReconciles),
	})

	// Build deliverer for triggering cluster reconciliations.
	s.clusterDeliverer = util.NewDelayingDeliverer()

	// Start informers on the resources for the federated type
	enqueueObj := s.worker.EnqueueObject

	targetNamespace := controllerConfig.TargetNamespace

	targetAPIResource := typeConfig.GetTargetType()
	s.federatedStore, s.federatedController = util.NewResourceInformer(federatedTypeClient, targetNamespace, &federatedAPIResource, enqueueObj)
	s.statusStore, s.statusController = util.NewResourceInformer(statusClient, targetNamespace, statusAPIResource, enqueueObj)

	// Federated informer for resources in member clusters
	s.informer, err = util.NewFederatedInformer(
		controllerConfig,
		client,
		&targetAPIResource,
		func(obj runtimeclient.Object) {
			qualifiedName := util.NewQualifiedName(obj)
			s.worker.EnqueueForRetry(qualifiedName)
		},
		&util.ClusterLifecycleHandlerFuncs{
			ClusterAvailable: func(cluster *fedv1b1.KubeFedCluster) {
				// When new cluster becomes available process all the target resources again.
				s.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(s.clusterAvailableDelay))
			},
			// When a cluster becomes unavailable process all the target resources again.
			ClusterUnavailable: func(cluster *fedv1b1.KubeFedCluster, _ []interface{}) {
				s.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(s.clusterUnavailableDelay))
			},
		},
	)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// minimizeLatency reduces delays and timeouts to make the controller more responsive (useful for testing).
func (s *KubeFedStatusController) minimizeLatency() {
	s.clusterAvailableDelay = time.Second
	s.clusterUnavailableDelay = time.Second
	s.smallDelay = 20 * time.Millisecond
	s.worker.SetDelay(50*time.Millisecond, s.clusterAvailableDelay)
}

// Run runs the status controller
func (s *KubeFedStatusController) Run(stopChan <-chan struct{}) {
	go s.federatedController.Run(stopChan)
	go s.statusController.Run(stopChan)
	s.informer.Start()
	s.clusterDeliverer.StartWithHandler(func(_ *util.DelayingDelivererItem) {
		s.reconcileOnClusterChange()
	})

	s.worker.Run(stopChan)

	// Ensure all goroutines are cleaned up when the stop channel closes
	go func() {
		<-stopChan
		s.informer.Stop()
		s.clusterDeliverer.Stop()
	}()
}

// Wait until all data stores are in sync for a definitive timeout, and returns if there is an error or a timeout.
func (s *KubeFedStatusController) waitForSync() error {
	return wait.PollUntilContextTimeout(context.TODO(), util.SyncedPollPeriod, s.cacheSyncTimeout, true, func(_ context.Context) (bool, error) {
		return s.isSynced(), nil
	})
}

// Check whether all data stores are in sync. False is returned if any of the informer/stores is not yet
// synced with the corresponding api server.
func (s *KubeFedStatusController) isSynced() bool {
	if !s.informer.ClustersSynced() {
		klog.V(2).Infof("Cluster list not synced")
		return false
	}
	if !s.federatedController.HasSynced() {
		klog.V(2).Infof("Federated type not synced")
		return false
	}
	if !s.statusController.HasSynced() {
		klog.V(2).Infof("Status not synced")
		return false
	}

	clusters, err := s.informer.GetReadyClusters()
	if err != nil {
		runtime.HandleError(errors.Wrap(err, "Failed to get ready clusters"))
		return false
	}
	if !s.informer.GetTargetStore().ClustersSynced(clusters) {
		klog.V(2).Info("Target clusters' informers not synced")
		return false
	}
	return true
}

// The function triggers reconciliation of all target federated resources.
func (s *KubeFedStatusController) reconcileOnClusterChange() {
	if !s.isSynced() {
		s.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(s.clusterAvailableDelay))
	}
	for _, obj := range s.federatedStore.List() {
		qualifiedName := util.NewQualifiedName(obj.(runtimeclient.Object))
		s.worker.EnqueueWithDelay(qualifiedName, s.smallDelay)
	}
}

func (s *KubeFedStatusController) reconcile(qualifiedName util.QualifiedName) util.ReconciliationStatus {
	if err := s.waitForSync(); err != nil {
		klog.Fatalf("failed to wait for all data stores to sync: %v", err)
	}

	federatedKind := s.typeConfig.GetFederatedType().Kind
	statusKind := s.typeConfig.GetStatusType().Kind
	key := qualifiedName.String()

	klog.V(4).Infof("Starting to reconcile %v %v", statusKind, key)
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished reconciling %v %v (duration: %v)", statusKind, key, time.Since(startTime))
		metrics.UpdateControllerReconcileDurationFromStart("statuscontroller", startTime)
	}()

	fedObject, err := s.objFromCache(s.federatedStore, federatedKind, key)
	if err != nil {
		return util.StatusError
	}

	if fedObject == nil || fedObject.GetDeletionTimestamp() != nil {
		klog.V(4).Infof("No federated type for %v %v found", federatedKind, key)
		// Status object is removed by GC. So we don't have to do anything more here.
		return util.StatusAllOK
	}

	clusterNames, err := s.clusterNames()
	if err != nil {
		runtime.HandleError(errors.Wrap(err, "Failed to get cluster list"))
		return util.StatusNotSynced
	}

	clusterStatus, err := s.clusterStatuses(clusterNames, key)
	if err != nil {
		return util.StatusError
	}

	existingStatus, err := s.objFromCache(s.statusStore, statusKind, key)
	if err != nil {
		return util.StatusError
	}

	resourceGroupVersion := schema.GroupVersion{Group: s.typeConfig.GetStatusType().Group, Version: s.typeConfig.GetStatusType().Version}
	federatedResource := util.FederatedResource{
		TypeMeta: metav1.TypeMeta{
			Kind:       s.typeConfig.GetStatusType().Kind,
			APIVersion: resourceGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      qualifiedName.Name,
			Namespace: qualifiedName.Namespace,
			// Add ownership of status object to corresponding
			// federated object, so that status object is deleted when
			// the federated object is deleted.
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: fedObject.GetAPIVersion(),
				Kind:       fedObject.GetKind(),
				Name:       fedObject.GetName(),
				UID:        fedObject.GetUID(),
			}},
		},
		ClusterStatus: clusterStatus,
	}
	status, err := util.GetUnstructured(federatedResource)
	if err != nil {
		klog.Errorf("Failed to convert to Unstructured: %s %q: %v", statusKind, key, err)
		return util.StatusError
	}

	if existingStatus == nil {
		_, err = s.statusClient.Resources(qualifiedName.Namespace).Create(context.Background(), status, metav1.CreateOptions{})
		if err != nil {
			runtime.HandleError(errors.Wrapf(err, "Failed to create status object for federated type %s %q", statusKind, key))
			return util.StatusNeedsRecheck
		}
	} else if !reflect.DeepEqual(existingStatus.Object["clusterStatus"], status.Object["clusterStatus"]) {
		if status.Object["clusterStatus"] == nil {
			status.Object["clusterStatus"] = make([]util.ResourceClusterStatus, 0)
		}
		existingStatus.Object["clusterStatus"] = status.Object["clusterStatus"]
		_, err = s.statusClient.Resources(qualifiedName.Namespace).Update(context.Background(), existingStatus, metav1.UpdateOptions{})
		if err != nil {
			runtime.HandleError(errors.Wrapf(err, "Failed to update status object for federated type %s %q", statusKind, key))
			return util.StatusNeedsRecheck
		}
	}

	return util.StatusAllOK
}

func (s *KubeFedStatusController) rawObjFromCache(store cache.Store, kind, key string) (runtimeclient.Object, error) {
	cachedObj, exist, err := store.GetByKey(key)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "Failed to query %s store for %q", kind, key)
		runtime.HandleError(wrappedErr)
		return nil, err
	}
	if !exist {
		return nil, nil
	}
	return cachedObj.(runtimeclient.Object).DeepCopyObject().(runtimeclient.Object), nil
}

func (s *KubeFedStatusController) objFromCache(store cache.Store, kind, key string) (*unstructured.Unstructured, error) {
	obj, err := s.rawObjFromCache(store, kind, key)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	return obj.(*unstructured.Unstructured), nil
}

func (s *KubeFedStatusController) clusterNames() ([]string, error) {
	clusters, err := s.informer.GetReadyClusters()
	if err != nil {
		return nil, err
	}
	clusterNames := []string{}
	for _, cluster := range clusters {
		clusterNames = append(clusterNames, cluster.Name)
	}

	return clusterNames, nil
}

// clusterStatuses returns the resource status in member cluster.
func (s *KubeFedStatusController) clusterStatuses(clusterNames []string, key string) ([]util.ResourceClusterStatus, error) {
	clusterStatus := []util.ResourceClusterStatus{}

	targetKind := s.typeConfig.GetTargetType().Kind
	for _, clusterName := range clusterNames {
		clusterObj, exist, err := s.informer.GetTargetStore().GetByKey(clusterName, key)
		if err != nil {
			wrappedErr := errors.Wrapf(err, "Failed to get %s %q from cluster %q", targetKind, key, clusterName)
			runtime.HandleError(wrappedErr)
			return nil, wrappedErr
		}

		var status map[string]interface{}
		if exist {
			clusterObj := clusterObj.(*unstructured.Unstructured)

			var found bool
			status, found, err = unstructured.NestedMap(clusterObj.Object, "status")
			if err != nil || !found {
				wrappedErr := errors.Wrapf(err, "Failed to get status of cluster resource object %s %q for cluster %q", targetKind, key, clusterName)
				runtime.HandleError(wrappedErr)
			}
		}
		resourceClusterStatus := util.ResourceClusterStatus{ClusterName: clusterName, Status: status}
		clusterStatus = append(clusterStatus, resourceClusterStatus)
	}

	sort.Slice(clusterStatus, func(i, j int) bool {
		return clusterStatus[i].ClusterName < clusterStatus[j].ClusterName
	})
	return clusterStatus, nil
}
