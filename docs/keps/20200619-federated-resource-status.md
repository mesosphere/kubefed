---
kep-number: 20200619
short-desc: Kubefed -- Federated Resources Status
title: Kubefed -- Federated Resources Status
authors:
  - "@hectorj2f"
reviewers:
  - "@irfan"
  - "@hectorj2f"
  - "@jimmidyson"
  - "@pmorie"
approvers:
- "@irfan"
- "@jimmidyson"
- "@pmorie"
editor: TBD
creation-date: 2020-06-19
last-updated: 2020-07-28
status: provisional
---

# Kubefed -- Federated Resources Status

## Table of Contents

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
- [Alternatives](#alternatives)
<!-- /toc -->

## Summary

Kubefed needs to improve its definition of status for the federated resources.
Users lack of proper visibility over the status of the federated
resources. For instance, if you federated a deployment the federated status should
report if the deployment failed or error at any time.

A federated resource only reflects the status of propagation actions, but it doesn't
reflect the status of whether the resource is running or failed.

## Motivation

Nowadays users have to connect to the kubefed clusters to be aware if the federated
resources are healthy or not across clusters.

### Goals

* Quickly identify unhealthy federated resources by relying on the status of the federated resources.
* Improve the troubleshooting of failures when propagating resources.
* Define a new api version potentially `v1beta2` to include these new fields.

### Non Goals

* Set the resource status for all the federated resources and clusters whenever a federated resource satisfies the condition `Ready=True`.
Addition of feature flags to control plane components to limit or throttle requests made to Kubernetes API servers akin to kube-api-qps or kube-api-burst in kubelet.

## Proposal

Kubefed reports a limited set of states related to the state of the federation
of resources themselves. This proposal aims to enrich the federated resource
status by adding the conditions and an extra field to the federated resource status.

### Conditions

```go
ClusterPropagationOK PropagationStatus = ""
WaitingForRemoval    PropagationStatus = "WaitingForRemoval"

// Cluster-specific errors
ClusterNotReady        PropagationStatus = "ClusterNotReady"
CachedRetrievalFailed  PropagationStatus = "CachedRetrievalFailed"
ComputeResourceFailed  PropagationStatus = "ComputeResourceFailed"
ApplyOverridesFailed   PropagationStatus = "ApplyOverridesFailed"
CreationFailed         PropagationStatus = "CreationFailed"
UpdateFailed           PropagationStatus = "UpdateFailed"
DeletionFailed         PropagationStatus = "DeletionFailed"
LabelRemovalFailed     PropagationStatus = "LabelRemovalFailed"
RetrievalFailed        PropagationStatus = "RetrievalFailed"
AlreadyExists          PropagationStatus = "AlreadyExists"
FieldRetentionFailed   PropagationStatus = "FieldRetentionFailed"
VersionRetrievalFailed PropagationStatus = "VersionRetrievalFailed"
ClientRetrievalFailed  PropagationStatus = "ClientRetrievalFailed"
ManagedLabelFalse      PropagationStatus = "ManagedLabelFalse"

CreationTimedOut     PropagationStatus = "CreationTimedOut"
UpdateTimedOut       PropagationStatus = "UpdateTimedOut"
DeletionTimedOut     PropagationStatus = "DeletionTimedOut"
LabelRemovalTimedOut PropagationStatus = "LabelRemovalTimedOut"

AggregateSuccess       AggregateReason = ""
ComputePlacementFailed AggregateReason = "ComputePlacementFailed"
NamespaceNotFederated  AggregateReason = "NamespaceNotFederated"

PropagationConditionType ConditionType = "Propagation"
```

The current federated resource properties don't help to track the status of the deployed resources in the kubefed clusters.

The idea is to extend current `GenericFederatedStatus/Clusters` with the Status of the resources per cluster:

```go

type GenericFederatedStatus struct {
  	ObservedGeneration  int64                  `json:"observedGeneration,omitempty"`
  	Conditions          []*GenericCondition    `json:"conditions,omitempty"`
  	Clusters            []GenericClusterStatus `json:"clusters,omitempty"`
}

type GenericFederatedResource struct {
	metav1.TypeMeta                `json:",inline"`
	metav1.ObjectMeta              `json:"metadata,omitempty"`

	Status *GenericFederatedStatus `json:"status,omitempty"`
}

type GenericClusterStatus struct {
	Name   string            `json:"name"`
	Status PropagationStatus `json:"status,omitempty"`

  Conditions []*metav1.Condition `json:"conditions,omitempty"`
}

```

The idea is to use the type `k8s.io/apimachinery/pkg/apis/meta/v1` for `Condition` that looks like:

```go
type Condition struct {
	// Type of condition in CamelCase or in foo.example.com/CamelCase.
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// +required
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// Status of the condition, one of True, False, Unknown.
	// +required
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// If set, this represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.condition[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`
	// Last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition in CamelCase.
	// The specific API may choose whether or not this field is considered a guaranteed API.
	// This field may not be empty.
	// +required
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// This field may be empty.
	// +required
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}
```

As of now `Conditions` field of a federated resource holds the status of the federated actions (aka propagation status).
In other words, it defines the conditions of the propagation status for a resource.

```yaml
- apiVersion: types.kubefed.io/v1beta1
  kind: FederatedDeployment
  metadata:
    finalizers:
    - kubefed.io/sync-controller
    generation: 1
    name: asystem
    namespace: asystem
    resourceVersion: "70174497"
  spec:
    placement:
      clusters:
      - name: cluster3
      - name: cluster2
      - name: cluster1
    template:
      metadata:
        labels:
          app: nginx
      spec:
        replicas: 3
        selector:
          matchLabels:
            app: nginx
        template:
          metadata:
            labels:
              app: nginx
          spec:
            containers:
            - image: nginx
              name: nginx
  status:
    conditions:
    - lastTransitionTime: "2020-05-25T19:47:59Z"
      lastUpdateTime: "2020-05-25T19:47:59Z"
      status: "True"
      type: Propagation
```

`status.conditions` reports the latest status which defines the state of the propagation.
Obviously it is not necessary to report all the clusters for which the propagation was completed
successfully.
In relation to this, this approach does not change the initial implementation where
the status is only collected for the services and in case of an error during the propagation.
Therefore the proposed solution updates the `status/conditions` and `clusters/conditions` to
identify the state of a federated resource in case of failure or non-ready state.

The intention in this proposal is to extend the current available `Clusters` status with an additional field `Conditions` to
hold the status of the federated resources, e.g Ready, NotReady.

The status of the federated resources should determine whether the resources satisfy
a `Ready` condition, and otherwise report their error status.
To do so, this property reports the status of the federated resources in their
target clusters whenever a `ReadyCondition` is not satisfied.
This condition would need to be identified per type or by the usage of an interface
`IsReady` that determines this value per type of resource.
By doing so, we ensure the `Clusters/Conditions` property shows the status of only unhealthy
resources for the respective clusters.

If we re-use the example from above and imagine a scenario where this `FederatedDeployment` resource remained `Ready=True` in two clusters, but crashed in `cluster3`.
The value of `status.clusters[<index>].conditions` should reflect the new state for that specific cluster.
The value of `status.conditions` defines the aggregated condition of the Federated resource itself.

```yaml
- apiVersion: types.kubefed.io/v1beta1
  kind: FederatedDeployment
  metadata:
    finalizers:
    - kubefed.io/sync-controller
    generation: 1
    name: asystem
    namespace: asystem
    resourceVersion: "70174497"
  spec:
    placement:
      clusters:
      - name: cluster2
      - name: cluster1
      - name: cluster3
    template:
      metadata:
        labels:
          app: nginx
      spec:
        replicas: 3
        selector:
          matchLabels:
            app: nginx
        template:
          metadata:
            labels:
              app: nginx
          spec:
            containers:
            - image: nginx
              name: nginx
  status:
    clusters:
    - name: "cluster3"
      status: "CreationFailed"
      conditions:
      - lastTransitionTime: "2020-05-25T20:23:59Z"
        lastUpdateTime: "2020-05-25T20:23:59Z"
        status: "True"
        type: "NotReady"
        reason: "ReplicaFailure"
        message: "1 Replica is not ready, you don't have enough resources"
    conditions:
    - lastTransitionTime: "2020-05-25T20:23:59Z"
      lastUpdateTime: "2020-05-25T20:23:59Z"
      status: "False"
      type: "Propagation"
```

The value of `status.conditions` describes the propagation error that happened to `cluster3`
while `clusters[0].conditions` for `name=cluster3` contains a `NotReady` condition type with a detailed
description of the state of the federated resource in `cluster3`.
The value of `status.clusters[0].status` reuses the current set of available status types, e.g. `"CreationFailed"`, which identifies the current problem during the propagation.


The status of the `FederatedDeployment` remains `Ready=True` in the rest
of clusters: `cluster1` and `cluster2`.
However as you can see, the `clusters[<index>].conditions` does not contain values with the `IsReady` condition for `cluster2` and `cluster1`.
The system omits the reporting of these `Ready=True` status due two reasons:

* easily decipher the propagation and other failures.
* reduce load on etcd, which goes directly proportional to all resources in all clusters.

If a federated resource does not have a status field, a successful creation/update would
reflect its readiness. Then the `ReadyCondition` would be satisfied by its creation.
For these resources the value of `Conditions` would rely on the value of the propagation state of that resource.
An example could be a `ClusterRole` resource that doesn't have a status property, but
kubefed nowadays reports if the propagation of that resource worked.

```yaml
apiVersion: types.kubefed.io/v1beta1
kind: FederatedClusterRole
metadata:
  name: test-clusterrole
spec:
  template:
    rules:
    - apiGroups:
      - '*'
      resources:
      - '*'
      verbs:
      - '*'
  placement:
    clusters:
    - name: cluster2
    - name: cluster1
status:
  conditions:
  - lastTransitionTime: "2020-05-25T19:47:59Z"
    lastUpdateTime: "2020-05-25T19:47:59Z"
    status: "True"
    type: Propagation
```

However, there is a problem with this approach, the status schema varies based on the custom
resource. Unfortunately that brings a problem when determining if a federated resource
is ready or not.

In the following, there is a list of Status objects of different custom resource definitions:

```go
type AddonStatus struct {
	Ready bool          `json:"ready" yaml:"ready"`
	Stage status.Status `json:"stage,omitempty" yaml:"stage,omitempty"`
}

type PodStatus struct {
  Phase PodPhase
 ...
 }

 type ServiceStatus struct {
  LoadBalancer LoadBalancerStatus
 }

// KonvoyClusterStatus defines the observed state of KonvoyCluster
type KonvoyClusterStatus struct {
	// Phase represents the current phase of Konvoy cluster actuation.
	// E.g. Pending, Provisioning, Provisioned, Deleting, Failed, etc.
	// +optional
	Phase KonvoyClusterPhase `json:"phase,omitempty"`

  ...
}
```

As mentioned, their Status schema is quite different from one to another.

Consequently, the intention is to **enforce** in the federated resources this approved [recommendation](https://github.com/kubernetes/enhancements/pull/1624/files).
to expect a common schema for `.status.conditions` and share golang logic for common Get, Set, Is for `.status.conditions`.

1. For all new APIs, have a common type for `.status.conditions`.
2. Provide common utility methods for `HasCondition`, `IsConditionTrue`, `SetCondition`, etc.
3. Provide recommended defaulting functions that set required fields and can be embedded into conversion/default functions.

By following this approach, kubefed would be able to properly consume and report the status
of any federated resource by checking the `status.conditions` (e.g `Ready=True`) fields.


**Note that**, this proposed solution might include additional flags to the kubefed control-plane
components to avoid blowing out the control-plane due to frequent and concurrent API calls to
update the status of the federated resources.

Obviously users might want to be able to enable/disable the definition of:

* Which condition to look for each federated resource to determine its readiness, e.g. `Ready=True`, `Deployed=True`.
This might be especially useful for custom resource types without a `IsReady` standard condition.

* When to show up the raw status of the resources or just the failed status (as today).
By raw status, the system understand to show the status of all the federated resources `Ready` and not `Ready`.
This can have an impact in the performance, so this should be configured with precautions.

To do so, the system exposes properties as part of each `FederatedTypeConfig` to define
the desired behavior at federated resource type.

### User customizable field

Extend the `FederatedTypeConfig` with an extra field, named `statusPath`, that contains a JSON path to the field of the `FederatedTypeConfig.spec.targetType` that should be rolled up to the federated resource.

```go
// FederatedTypeConfigSpec defines the desired state of FederatedTypeConfig.
type FederatedTypeConfigSpec struct {
  // The configuration of the target type. If not set, the pluralName and
  // groupName fields will be set from the metadata.name of this resource. The
  // kind field must be set.
  TargetType APIResource `json:"targetType"`
  // Whether or not propagation to member clusters should be enabled.
  Propagation PropagationMode `json:"propagation"`
  // Configuration for the federated type that defines (via
  // template, placement and overrides fields) how the target type
  // should appear in multiple cluster.
  FederatedType APIResource `json:"federatedType"`
  // Configuration for the status type that holds information about which type
  // holds the status of the federated resource. If not provided, the group
  // and version will default to those provided for the federated type api
  // resource.
  // +optional
  StatusType *APIResource `json:"statusType,omitempty"`
  // Whether or not Status object should be populated.
  // +optional
  StatusCollection *StatusCollectionMode `json:"statusCollection,omitempty"`

  // A JSON path to the field in the TargetType that should be rolled up to
  // the management cluster.
  // +optional
  StatusPath string `json:"statusPath,omitempty"`
}
```

If `StatusCollection` is not `Enabled` then the status of the federated resource
should not be rolled up back to the management cluster.

An example to rollup the number of replicas of a `Deployment`:

```yaml
apiVersion: types.kubefed.io/v1beta1
kind: FederatedTypeConfig
spec:
  federatedType:
    group: types.kubefed.io
    kind: FederatedDeployment
    pluralName: federateddeployments
    scope: Namespaced
    version: v1beta1
  propagation: Enabled
  targetType:
    group: apps
    kind: Deployment
    pluralName: deployments
    scope: Namespaced
    version: v1
  #
  # statusPath is a JSON path to the field of the targetType that should be
  # rolled up to the management cluster. In this example we want to roll up
  # the deployed replicas.
  #
  statusPath: ".status.replicas"
```

In this proposal, the value will be stored in a new field, named `statusRemote`, in `GenericClusterStatus`.


```go

type GenericFederatedStatus struct {
  	ObservedGeneration  int64                  `json:"observedGeneration,omitempty"`
  	Conditions          []*GenericCondition    `json:"conditions,omitempty"`
  	Clusters            []GenericClusterStatus `json:"clusters,omitempty"`
}

type GenericFederatedResource struct {
	metav1.TypeMeta                `json:",inline"`
	metav1.ObjectMeta              `json:"metadata,omitempty"`

	Status *GenericFederatedStatus `json:"status,omitempty"`
}

type GenericClusterStatus struct {
	Name   string            `json:"name"`
	Status PropagationStatus `json:"status,omitempty"`

  Conditions []*metav1.Condition `json:"conditions,omitempty"`
  StatusRemote string `json:"statusRemote,omitempty"`
}

```

Example of how the FederatedDeployment can look like:

```yaml
apiVersion: types.kubefed.io/v1beta1
kind: FederatedDeployment
metadata:
  <...snip...>
spec:
  <...snip...>
status:
  clusters:
  - name: "cluster3"
    status: "CreationFailed"
    #
    # the value of the JSON path of the corresponding deployment in cluster-3
    #
    statusRemote: "0"
  conditions:
  - lastTransitionTime: "2020-05-25T20:23:59Z"
    lastUpdateTime: "2020-05-25T20:23:59Z"
    status: "False"
    type: "Propagation"
```

#### Updating federated resource status

Kubefed already has [code](https://github.com/kubernetes-sigs/kubefed/blob/79cb7e20d3d388ee8a5c12e3ed7cdd51a8b14b99/pkg/controller/status/controller.go#L143) to cache the status of resources in the member clusters. We can extend it to update the `statusRemote` field in the federated resource when the reconcile loop is called.

### User Stories

#### Story 1

Users create federated resources and want to be aware of their status without having
to access to the remote clusters.

In the following example, we have a `FederatedAddon`, named `reloader`, deployed across ten `kubefedclusters`.
An `Addon` is a custom resource definition that abstract the creation of apps composed of
one or multiple Helm charts.

```yaml
---
apiVersion: types.kubefed.io/v1beta1
kind: FederatedAddon
metadata:
  name: reloader
  namespace: kubeaddons
spec:
  placement:
    clusters:
    - name: cluster10
    - name: cluster9
    - name: cluster8
    - name: cluster7
    - name: cluster6
    - name: cluster5
    - name: cluster4
    - name: cluster3
    - name: cluster2
    - name: cluster1
  template:
    metadata:
      labels:
        kubeaddons.mesosphere.io/name: reloader
    spec:
      chartReference:
        chart: reloader
        repo: https://stakater.github.io/stakater-charts
        values: |
          ---
          reloader:
            deployment:
              resources:
                limits:
                  cpu: "100m"
                  memory: "512Mi"
                requests:
                  cpu: "100m"
                  memory: "128Mi"
        version: v0.0.49
      kubernetes:
        minSupportedVersion: v1.15.6
status:
  conditions:
  - lastTransitionTime: "2020-05-25T19:47:59Z"
    lastUpdateTime: "2020-05-25T19:47:59Z"
    status: "True"
    type: Propagation
```

At any specific time, this `FederatedAddon` crashed on three clusters.
As a consequence, the value of its `status` should look similar to this:

```yaml
---
apiVersion: types.kubefed.io/v1beta1
kind: FederatedAddon
metadata:
  name: reloader
  namespace: kubeaddons
spec:
  placement:
    clusters:
    - name: cluster10
    - name: cluster9
    - name: cluster8
    - name: cluster7
    - name: cluster6
    - name: cluster5
    - name: cluster4
    - name: cluster3
    - name: cluster2
    - name: cluster1
  template:
    metadata:
      labels:
        kubeaddons.mesosphere.io/name: reloader
    spec:
      chartReference:
        chart: reloader
        repo: https://stakater.github.io/stakater-charts
        values: |
          ---
          reloader:
            deployment:
              resources:
                limits:
                  cpu: "100m"
                  memory: "512Mi"
                requests:
                  cpu: "100m"
                  memory: "128Mi"
        version: v0.0.49
      kubernetes:
        minSupportedVersion: v1.15.6
status:
  status:
    clusters:
    - name: "cluster1"
      conditions:
      - lastTransitionTime: "2020-05-25T20:23:59Z"
        lastUpdateTime: "2020-05-25T20:23:59Z"
        status: "True"
        type: "NotReady"
        reason: "Failed"
    - name: "cluster2"
      conditions:
      - lastTransitionTime: "2020-05-25T20:23:59Z"
        lastUpdateTime: "2020-05-25T20:23:59Z"
        status: "True"
        type: "NotReady"
        reason: "Failed"
    - name: "cluster3"
      conditions:
      - lastTransitionTime: "2020-05-25T20:23:59Z"
        lastUpdateTime: "2020-05-25T20:23:59Z"
        status: "True"
        type: "NotReady"
        reason: "Failed"
  conditions:
  - lastTransitionTime: "2020-05-25T19:47:59Z"
    lastUpdateTime: "2020-05-25T19:47:59Z"
    status: "True"
    type: NotReady
```

The value of `status.clusters[<index>].conditions` could be extracted from the status of an addon whose `status.stage` is `Failed` and
`status.ready` value is `false`.
The ideal scenario would assume `message` and `reason` are available fields in a `conditions` property of the addon resource, so a detailed description
of the problem can be shared with the users.


## Alternatives

Another approach consists on segregating the propagation status and individual cluster resource status as two separate sub status fields in the status resource.

This approach would provide an option to define which status to show up between the propagation and cluster resource status.
This option could be added to the `FederatedTypeConfig` API type to specify which status to show/fetch at resource level.

Likewise, as mentioned above, the system only stores the failure status of the federated resources.
However a feature gate could enable in the control-plane for the collection of the raw status of all the resources side by side with the propagation status.
