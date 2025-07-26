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

package controller

import (
	"context"
	"fmt"
	"reflect"

	cachev1 "github.com/IguoChan/redis-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	RedisPort = 6379
)

const (
	RecordReasonFailed  = "Failed"
	RecordReasonWaiting = "Waiting"
)

//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redis/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 获取 Redis 实例
	redis := &cachev1.Redis{}
	if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 确保 StatefulSet 存在
	if err := r.reconcileStatefulSet(ctx, redis); err != nil {
		logger.Error(err, "reconcileStatefulSet failed")
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "reconcileStatefulSet failed: %s", err.Error())
		return ctrl.Result{}, r.updateStatus(ctx, redis)
	}

	// 确保 Service 存在
	if err := r.reconcileService(ctx, redis); err != nil {
		logger.Error(err, "reconcileService failed")
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "reconcileService failed: %s", err.Error())
		return ctrl.Result{}, r.updateStatus(ctx, redis)
	}

	if err := r.reconcileExporterService(ctx, redis); err != nil {
		logger.Error(err, "reconcileExporterService failed")
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "reconcileExporterService failed: %s", err.Error())
		return ctrl.Result{}, r.updateStatus(ctx, redis)
	}

	// 确保 PVC 存在
	if err := r.reconcilePVC(ctx, redis); err != nil {
		logger.Error(err, "reconcilePVC failed")
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "reconcilePVC failed: %s", err.Error())
		return ctrl.Result{}, r.updateStatus(ctx, redis)
	}

	// 更新状态
	return ctrl.Result{}, r.updateStatus(ctx, redis)
}

func (r *RedisReconciler) reconcileStatefulSet(ctx context.Context, redis *cachev1.Redis) error {
	logger := log.FromContext(ctx)

	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-redis", redis.Name),
			Namespace: redis.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: fmt.Sprintf("%s-svc", redis.Name),
			Replicas:    pointer.Int32(1), // Standalone 模式
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "redis",
					"name": redis.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":  "redis",
						"name": redis.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           redis.Spec.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: RedisPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "redis-data",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "redis-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: redis.Spec.Storage.Size,
							},
						},
						StorageClassName: pointer.String("redis-storage"),
					},
				},
			},
		},
	}

	if redis.Spec.Exporter.Enable {
		// 添加 Redis Exporter 容器
		desired.Spec.Template.Spec.Containers = append(desired.Spec.Template.Spec.Containers, corev1.Container{
			Name:            "redis-exporter",
			Image:           redis.Spec.Exporter.Image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports: []corev1.ContainerPort{
				{
					Name:          "metrics",
					ContainerPort: redis.Spec.Exporter.Port,
				},
			},
			Args: []string{
				fmt.Sprintf("--redis.addr=localhost:%d", RedisPort),
				fmt.Sprintf("--web.listen-address=:%d", redis.Spec.Exporter.Port),
			},
			Resources: redis.Spec.Exporter.Resources,
		})
	}

	// 设置 OwnerReference
	if err := ctrl.SetControllerReference(redis, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)

	if errors.IsNotFound(err) {
		logger.Info("Creating StatefulSet", "name", desired.Name)
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// 更新 StatefulSet (简化处理)
	logger.Info("Updating StatefulSet", "name", desired.Name)
	desired.Spec.DeepCopyInto(&existing.Spec)
	return r.Update(ctx, existing)
}

func (r *RedisReconciler) reconcileService(ctx context.Context, redis *cachev1.Redis) error {
	logger := log.FromContext(ctx)

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-svc", redis.Name),
			Namespace: redis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app":  "redis",
				"name": redis.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       RedisPort,
					TargetPort: intstr.FromInt(RedisPort),
					NodePort:   redis.Spec.NodePort,
				},
			},
		},
	}

	// 设置 OwnerReference
	if err := ctrl.SetControllerReference(redis, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)

	if errors.IsNotFound(err) {
		logger.Info("Creating Service", "name", desired.Name)
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// 更新 Service (只更新端口)
	if existing.Spec.Ports[0].NodePort != desired.Spec.Ports[0].NodePort {
		logger.Info("Updating Service port", "name", desired.Name)
		existing.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, existing)
	}

	return nil
}

func (r *RedisReconciler) reconcileExporterService(ctx context.Context, redis *cachev1.Redis) error {
	if !redis.Spec.Exporter.Enable {
		return nil
	}

	logger := log.FromContext(ctx)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-exporter", redis.Name),
			Namespace: redis.Namespace,
			Labels: map[string]string{
				"app":  "redis",
				"name": redis.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":  "redis",
				"name": redis.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       redis.Spec.Exporter.Port,
					TargetPort: intstr.FromInt(int(redis.Spec.Exporter.Port)),
				},
			},
		},
	}

	// 设置 OwnerReference
	if err := ctrl.SetControllerReference(redis, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}, existing)

	if errors.IsNotFound(err) {
		logger.Info("Creating Exporter Service", "name", desired.Name)
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// 更新 Service（如果需要）
	if !reflect.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Ports = desired.Spec.Ports
		logger.Info("Updating Exporter Service", "name", desired.Name)
		return r.Update(ctx, existing)
	}

	return nil
}

func (r *RedisReconciler) reconcilePVC(ctx context.Context, redis *cachev1.Redis) error {
	logger := log.FromContext(ctx)

	pvName := fmt.Sprintf("%s-pv", redis.Name)

	// 创建 PV (使用 HostPath)
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: redis.Spec.Storage.Size,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRecycle,
			StorageClassName:              "redis-storage",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: redis.Spec.Storage.HostPath,
				},
			},
		},
	}

	if err := r.Create(ctx, pv); err != nil && !errors.IsAlreadyExists(err) {
		logger.Error(err, "create pv failed")
		return err
	}

	// 更新 PVC 指向创建的 PV (在 StatefulSet 中会自动创建 PVC)
	return nil
}

func (r *RedisReconciler) updateStatus(ctx context.Context, redis *cachev1.Redis) error {
	// 1. 获取关联的Pod
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		"name": redis.Name,
	}); err != nil {
		return err
	}

	// 2. 检查Pod状态
	if len(podList.Items) == 0 {
		r.Recorder.Event(redis, corev1.EventTypeNormal, RecordReasonWaiting, "no pod is ready")
		redis.Status.Phase = cachev1.RedisPhasePending
		return r.Status().Update(ctx, redis)
	}

	// 3. 确保所有Pod都处于Ready状态
	allPodsReady := true
	for _, pod := range podList.Items {
		isPodReady := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				isPodReady = true
				break
			}
		}
		if !isPodReady {
			allPodsReady = false
			break
		}
	}

	// 4. 如果Pod没准备好，直接返回
	if !allPodsReady {
		r.Recorder.Event(redis, corev1.EventTypeNormal, RecordReasonWaiting, "not every pod is ready")
		redis.Status.Phase = cachev1.RedisPhasePending
		return r.Status().Update(ctx, redis)
	}

	// 5. 获取服务信息
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-svc", redis.Name),
		Namespace: redis.Namespace,
	}, svc); err != nil {
		// 处理服务不存在的情况
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "get service failed: %s", err.Error())
		redis.Status.Phase = cachev1.RedisPhaseError
		return r.Status().Update(ctx, redis)
	}

	// 6. 验证Endpoint可用性
	endpoints := &corev1.Endpoints{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}, endpoints); err != nil {
		// 处理Endpoint不存在的情况
		r.Recorder.Eventf(redis, corev1.EventTypeWarning, RecordReasonFailed, "get endpoints failed: %s", err.Error())
		redis.Status.Phase = cachev1.RedisPhaseError
		return r.Status().Update(ctx, redis)
	}

	// 7. 确保Endpoint有可用的目标
	if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
		r.Recorder.Event(redis, corev1.EventTypeWarning, RecordReasonFailed, "endpoints have no address")
		redis.Status.Phase = cachev1.RedisPhaseError
		return r.Status().Update(ctx, redis)
	}

	// 8. 获取节点IP（从Endpoint获取）
	nodeIP := endpoints.Subsets[0].Addresses[0].IP
	endpoint := fmt.Sprintf("%s:%d", nodeIP, svc.Spec.Ports[0].TargetPort.IntVal)

	// 9. 成功更新状态
	redis.Status.Endpoint = endpoint
	redis.Status.Phase = cachev1.RedisPhaseReady

	return r.Status().Update(ctx, redis)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1.Redis{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
