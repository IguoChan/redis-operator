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
	MinStorageSize = "1Gi"  // 最小存储大小
	MaxStorageSize = "10Gi" // 最大存储大小
)

// log is for logging in this package.
var redislog = logf.Log.WithName("redis-resource")

func (r *Redis) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-cache-iguochan-io-v1-redis,mutating=true,failurePolicy=fail,sideEffects=None,groups=cache.iguochan.io,resources=redis,verbs=create;update,versions=v1,name=mredis.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Redis{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Redis) Default() {
	redislog.Info("default", "name", r.Name)

	// 设置默认镜像
	if r.Spec.Image == "" {
		r.Spec.Image = "redis:7.0"
		redislog.Info("Setting default image", "image", r.Spec.Image)
	}

	// 设置默认端口
	if r.Spec.NodePort == 0 {
		r.Spec.NodePort = 31000
		redislog.Info("Setting default NodePort", "nodePort", r.Spec.NodePort)
	} // 设置默认存储路径
	if r.Spec.Storage.HostPath == "" {
		r.Spec.Storage.HostPath = "/data"
		redislog.Info("Setting default host path", "hostPath", r.Spec.Storage.HostPath)
	}

	// 设置默认存储大小
	if r.Spec.Storage.Size.IsZero() {
		size := resource.MustParse("1Gi")
		r.Spec.Storage.Size = size
		redislog.Info("Setting default storage size", "size", size.String())
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-cache-iguochan-io-v1-redis,mutating=false,failurePolicy=fail,sideEffects=None,groups=cache.iguochan.io,resources=redis,verbs=create;update,versions=v1,name=vredis.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Redis{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateCreate() error {
	redislog.Info("validate create", "name", r.Name)

	return r.validateRedis(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateUpdate(old runtime.Object) error {
	redislog.Info("validate update", "name", r.Name)

	oldRedis, ok := old.(*Redis)
	if !ok {
		return fmt.Errorf("expected a Redis object but got %T", old)
	}

	if err := r.validateRedis(r); err != nil {
		return err
	}

	// 验证禁止修改的字段
	if oldRedis.Spec.Image != r.Spec.Image {
		return field.Forbidden(
			field.NewPath("spec", "image"),
			"image cannot be changed after creation",
		)
	}

	if oldRedis.Spec.Storage.HostPath != r.Spec.Storage.HostPath {
		return field.Forbidden(
			field.NewPath("spec", "storage", "hostPath"),
			"hostPath cannot be changed after creation",
		)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Redis) ValidateDelete() error {
	redislog.Info("validate delete", "name", r.Name)

	return nil
}

// validateRedis 执行具体验证逻辑
func (r *Redis) validateRedis(redis *Redis) error {
	allErrs := field.ErrorList{}

	// 验证存储大小范围
	minSize := resource.MustParse(MinStorageSize)
	maxSize := resource.MustParse(MaxStorageSize)

	if redis.Spec.Storage.Size.Cmp(minSize) < 0 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "storage", "size"),
			redis.Spec.Storage.Size.String(),
			fmt.Sprintf("storage size must be at least %s", MinStorageSize),
		))
	}

	if redis.Spec.Storage.Size.Cmp(maxSize) > 0 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "storage", "size"),
			redis.Spec.Storage.Size.String(),
			fmt.Sprintf("storage size must be no more than %s", MaxStorageSize),
		))
	}

	// 验证端口范围
	if redis.Spec.NodePort < 30000 || redis.Spec.NodePort > 32767 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "nodePort"),
			redis.Spec.NodePort,
			"nodePort must be between 30000 and 32767",
		))
	}

	// 验证主机路径安全
	if !isValidHostPath(redis.Spec.Storage.HostPath) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "storage", "hostPath"),
			redis.Spec.Storage.HostPath,
			"invalid host path, only /data directory is allowed",
		))
	}

	// 验证Redis Exporter
	if redis.Spec.Exporter.Enable {
		if redis.Spec.Exporter.Image == "" {
			redis.Spec.Exporter.Image = "oliver006/redis_exporter:v1.50.0"
			redislog.Info("Setting default exporter image", "image", redis.Spec.Exporter.Image)
		}

		if redis.Spec.Exporter.Port == 0 {
			redis.Spec.Exporter.Port = 9121
			redislog.Info("Setting default exporter port", "port", redis.Spec.Exporter.Port)
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs.ToAggregate()
}

// 验证主机路径安全的辅助函数
func isValidHostPath(path string) bool {
	// 这里简化处理，实际应根据安全策略制定
	allowedPaths := []string{"/data", "/data/", "/mnt/data"}
	for _, p := range allowedPaths {
		if path == p {
			return true
		}
	}
	return false
}
