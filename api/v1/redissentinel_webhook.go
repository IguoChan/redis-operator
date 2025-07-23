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
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	MinRedisSize    = "1Gi"   // Redis 最小存储大小
	MaxRedisSize    = "100Gi" // Redis 最大存储大小
	MinSentinelSize = "100Mi" // Sentinel 最小存储大小
	MaxSentinelSize = "1Gi"   // Sentinel 最大存储大小
	MinReplicas     = 3       // 最小副本数
	MaxReplicas     = 7       // 最大副本数
	MinQuorum       = 2       // Sentinel 最小仲裁数
)

// log is for logging in this package.
var redissentinellog = logf.Log.WithName("redissentinel-resource")

func (r *RedisSentinel) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-cache-iguochan-io-v1-redissentinel,mutating=true,failurePolicy=fail,sideEffects=None,groups=cache.iguochan.io,resources=redissentinels,verbs=create;update,versions=v1,name=mredissentinel.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RedisSentinel{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *RedisSentinel) Default() {
	redissentinellog.Info("default", "name", r.Name)

	// 设置默认Redis镜像
	if r.Spec.Image == "" {
		r.Spec.Image = "redis:7.0"
		redissentinellog.Info("Setting default Redis image", "image", r.Spec.Image)
	}

	// 设置默认Sentinel镜像
	if r.Spec.SentinelImage == "" {
		r.Spec.SentinelImage = "redis:7.0"
		redissentinellog.Info("Setting default Sentinel image", "image", r.Spec.SentinelImage)
	}

	// 设置默认RedisMaster端口
	if r.Spec.MasterNodePort == 0 {
		r.Spec.MasterNodePort = 30999
		redissentinellog.Info("Setting default Redis Master NodePort", "nodePort", r.Spec.MasterNodePort)
	}

	// 设置默认Redis端口
	if r.Spec.NodePort == 0 {
		r.Spec.NodePort = 31000
		redissentinellog.Info("Setting default Redis NodePort", "nodePort", r.Spec.NodePort)
	}

	// 设置默认Sentinel端口
	if r.Spec.SentinelNodePort == 0 {
		r.Spec.SentinelNodePort = 31001
		redissentinellog.Info("Setting default Sentinel NodePort", "sentinelNodePort", r.Spec.SentinelNodePort)
	}

	// 设置默认Redis副本数
	if r.Spec.RedisReplicas == 0 {
		r.Spec.RedisReplicas = 3
		redissentinellog.Info("Setting default Redis replicas", "redisReplicas", r.Spec.RedisReplicas)
	}

	// 设置默认Sentinel副本数
	if r.Spec.SentinelReplicas == 0 {
		r.Spec.SentinelReplicas = 3
		redissentinellog.Info("Setting default Sentinel replicas", "sentinelReplicas", r.Spec.SentinelReplicas)
	}

	// 设置默认存储路径
	if r.Spec.Storage.HostPath == "" {
		r.Spec.Storage.HostPath = "/data"
		redissentinellog.Info("Setting default host path", "hostPath", r.Spec.Storage.HostPath)
	}

	// 设置默认Redis存储大小
	if r.Spec.Storage.Size.IsZero() {
		size := resource.MustParse("1Gi")
		r.Spec.Storage.Size = size
		redissentinellog.Info("Setting default Redis storage size", "size", size.String())
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-cache-iguochan-io-v1-redissentinel,mutating=false,failurePolicy=fail,sideEffects=None,groups=cache.iguochan.io,resources=redissentinels,verbs=create;update,versions=v1,name=vredissentinel.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RedisSentinel{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisSentinel) ValidateCreate() error {
	redissentinellog.Info("validate create", "name", r.Name)

	return r.validateRedisSentinel()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *RedisSentinel) ValidateUpdate(old runtime.Object) error {
	redissentinellog.Info("validate update", "name", r.Name)

	oldSentinel, ok := old.(*RedisSentinel)
	if !ok {
		return fmt.Errorf("expected a RedisSentinel object but got %T", old)
	}

	if err := r.validateRedisSentinel(); err != nil {
		return err
	}

	// 验证禁止修改的字段
	if oldSentinel.Spec.Image != r.Spec.Image {
		return field.Forbidden(
			field.NewPath("spec", "image"),
			"Redis image cannot be changed after creation",
		)
	}

	if oldSentinel.Spec.SentinelImage != r.Spec.SentinelImage {
		return field.Forbidden(
			field.NewPath("spec", "sentinelImage"),
			"Sentinel image cannot be changed after creation",
		)
	}

	if oldSentinel.Spec.Storage.HostPath != r.Spec.Storage.HostPath {
		return field.Forbidden(
			field.NewPath("spec", "storage", "hostPath"),
			"hostPath cannot be changed after creation",
		)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RedisSentinel) ValidateDelete() error {
	redissentinellog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// validateRedisSentinel 执行所有验证逻辑
func (r *RedisSentinel) validateRedisSentinel() error {
	allErrs := field.ErrorList{}

	// 验证Redis存储大小范围
	if err := validateStorageSize(
		r.Spec.Storage.Size,
		MinRedisSize,
		MaxRedisSize,
		field.NewPath("spec", "storage", "size"),
		"Redis"); err != nil {
		allErrs = append(allErrs, err)
	}

	// 验证Sentinel副本数
	if r.Spec.SentinelReplicas < MinReplicas {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "sentinelReplicas"),
			r.Spec.SentinelReplicas,
			fmt.Sprintf("Sentinel replicas must be at least %d", MinReplicas),
		))
	} else if r.Spec.SentinelReplicas > MaxReplicas {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "sentinelReplicas"),
			r.Spec.SentinelReplicas,
			fmt.Sprintf("Sentinel replicas must be no more than %d", MaxReplicas),
		))
	}

	// 验证Redis副本数
	if r.Spec.RedisReplicas < MinReplicas {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "redisReplicas"),
			r.Spec.RedisReplicas,
			fmt.Sprintf("Redis replicas must be at least %d", MinReplicas),
		))
	} else if r.Spec.RedisReplicas > MaxReplicas {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "redisReplicas"),
			r.Spec.RedisReplicas,
			fmt.Sprintf("Redis replicas must be no more than %d", MaxReplicas),
		))
	}

	// 验证Sentinel仲裁数要求
	if r.Spec.SentinelReplicas < MinQuorum*2-1 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "sentinelReplicas"),
			r.Spec.SentinelReplicas,
			fmt.Sprintf("Sentinel replicas must be at least %d for a quorum of %d", MinQuorum*2-1, MinQuorum),
		))
	}

	// 验证RedisMaster端口范围
	if r.Spec.MasterNodePort < 30000 || r.Spec.MasterNodePort > 32767 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "nodePort"),
			r.Spec.MasterNodePort,
			"Redis master nodePort must be between 30000 and 32767",
		))
	}

	// 验证Redis端口范围
	if r.Spec.NodePort < 30000 || r.Spec.NodePort > 32767 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "nodePort"),
			r.Spec.NodePort,
			"Redis nodePort must be between 30000 and 32767",
		))
	}

	// 验证Sentinel端口范围
	if r.Spec.SentinelNodePort < 30000 || r.Spec.SentinelNodePort > 32767 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "sentinelNodePort"),
			r.Spec.SentinelNodePort,
			"Sentinel nodePort must be between 30000 and 32767",
		))
	}

	// 验证主机路径安全
	if !isValidHostPath(r.Spec.Storage.HostPath) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "storage", "hostPath"),
			r.Spec.Storage.HostPath,
			"invalid host path, only /data directory is allowed",
		))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs.ToAggregate()
}

// 辅助函数：验证存储大小
func validateStorageSize(size resource.Quantity, min, max string, path *field.Path, resourceType string) *field.Error {
	minSize := resource.MustParse(min)
	maxSize := resource.MustParse(max)

	if size.Cmp(minSize) < 0 {
		return field.Invalid(
			path,
			size.String(),
			fmt.Sprintf("%s storage size must be at least %s", resourceType, min),
		)
	}

	if size.Cmp(maxSize) > 0 {
		return field.Invalid(
			path,
			size.String(),
			fmt.Sprintf("%s storage size must be no more than %s", resourceType, max),
		)
	}

	return nil
}

// 验证仲裁数量的辅助函数
func validateSentinelQuorum(replicas int32) bool {
	// Sentinel需要大多数节点在线才能选举
	// 公式：大多数节点 = (replicas/2) + 1
	return replicas >= MinQuorum && replicas%2 == 1
}
