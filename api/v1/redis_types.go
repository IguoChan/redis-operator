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

type RedisPhase string

const (
	RedisPhasePending = "Pending"
	RedisPhaseError   = "Error"
	RedisPhaseReady   = "Ready"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisSpec defines the desired state of Redis
type RedisSpec struct {
	// Image: Redis Image
	// +kubebuilder:default="redis:7.0"
	Image string `json:"image,omitempty"`

	// NodePort Service
	// +kubebuilder:validation:Minimum=30000
	// +kubebuilder:validation:Maximum=32767
	// +kubebuilder:default=31000
	NodePort int32 `json:"nodePort,omitempty"`

	// 存储配置
	Storage StorageSpec `json:"storage,omitempty"`
}

// StorageSpec 定义 Redis 存储配置
type StorageSpec struct {
	// 存储大小
	// +kubebuilder:default="1Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// 主机目录路径
	// +kubebuilder:default="/data"
	HostPath string `json:"hostPath,omitempty"`
}

// RedisStatus defines the observed state of Redis
type RedisStatus struct {
	// 部署状态
	Phase RedisPhase `json:"phase,omitempty"`

	// Redis 服务端点
	Endpoint string `json:"endpoint,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".status.phase",name=phase,type=string,description="当前阶段"
//+kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint",description="访问端点"
//+kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image",description="使用的镜像"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="创建时间"

// Redis is the Schema for the redis API
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec,omitempty"`
	Status RedisStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RedisList contains a list of Redis
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
