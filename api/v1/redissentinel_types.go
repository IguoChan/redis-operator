/*
Copyright 2025.

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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisSentinelSpec defines the desired state of RedisSentinel
type RedisSentinelSpec struct {
	// Image: Redis Image
	// +kubebuilder:default="redis:7.0"
	Image string `json:"image,omitempty"`

	// SentinelImage: Sentinel Image
	// +kubebuilder:default="redis:7.0"
	SentinelImage string `json:"sentinelImage,omitempty"`

	// RedisReplicas: Number of Redis instances
	// +kubebuilder:default=3
	RedisReplicas int32 `json:"redisReplicas,omitempty"`

	// SentinelReplicas: Number of Sentinel instances
	// +kubebuilder:default=3
	SentinelReplicas int32 `json:"sentinelReplicas,omitempty"`

	// NodePort: Redis NodePort for external access
	// +kubebuilder:validation:Minimum=30000
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:default=30999
	MasterNodePort int32 `json:"masterNodePort,omitempty"`

	// NodePort: Redis NodePort for external access
	// +kubebuilder:validation:Minimum=30000
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:default=31000
	NodePort int32 `json:"nodePort,omitempty"`

	// SentinelNodePort: Sentinel NodePort for external access
	// +kubebuilder:validation:Minimum=30000
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:default=31001
	SentinelNodePort int32 `json:"sentinelNodePort,omitempty"`

	// Storage configuration
	Storage RedisSentinelStorageSpec `json:"storage,omitempty"`
}

// RedisSentinelStorageSpec defines storage configuration for RedisSentinel
type RedisSentinelStorageSpec struct {
	// Storage size
	// +kubebuilder:default="1Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// Host path directory
	// +kubebuilder:default="/data"
	HostPath string `json:"hostPath,omitempty"`
}

// RedisSentinelStatus defines the observed state of RedisSentinel
type RedisSentinelStatus struct {
	// Deployment phase
	Phase RedisPhase `json:"phase,omitempty"`

	// Redis endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// Sentinel endpoint
	SentinelEndpoint string `json:"sentinelEndpoint,omitempty"`

	// Master node name
	Master string `json:"master,omitempty"`

	LastRoleUpdateTime metav1.Time `json:"lastRoleUpdateTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=redissentinels,scope=Namespaced,shortName=rss
//+kubebuilder:printcolumn:JSONPath=".status.phase",name=phase,type=string,description="Current phase"
//+kubebuilder:printcolumn:name="RedisEndpoint",type="string",JSONPath=".status.endpoint",description="Redis endpoint"
//+kubebuilder:printcolumn:name="SentinelEndpoint",type="string",JSONPath=".status.sentinelEndpoint",description="Sentinel endpoint"
//+kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image",description="Redis image"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Creation time"

// RedisSentinel is the Schema for the redissentinels API
type RedisSentinel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSentinelSpec   `json:"spec,omitempty"`
	Status RedisSentinelStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisSentinelList contains a list of RedisSentinel
type RedisSentinelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisSentinel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisSentinel{}, &RedisSentinelList{})
}
