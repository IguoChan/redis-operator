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
	"strings"
	"time"

	cachev1 "github.com/IguoChan/redis-operator/api/v1"
	"github.com/go-redis/redis"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

// RedisSentinelReconciler reconciles a RedisSentinel object
type RedisSentinelReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	MasterPort   = 6378
	SentinelPort = 26379
)

//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redissentinels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redissentinels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.iguochan.io,resources=redissentinels/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=create;get;list;update;watch;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *RedisSentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling RedisSentinel", "request", req.NamespacedName)

	// Fetch the RedisSentinel instance
	redisSentinel := &cachev1.RedisSentinel{}
	if err := r.Get(ctx, req.NamespacedName, redisSentinel); err != nil {
		if errors.IsNotFound(err) {
			// CR deleted, ignore
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// 检查是否有 Pod 被删除
	if r.checkPodDeletion(ctx, redisSentinel) {
		logger.Info("Detected pod deletion, waiting for failover")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Reconcile Redis StatefulSet
	if err := r.reconcileRedisStatefulSet(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Redis StatefulSet")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// Reconcile Sentinel StatefulSet
	if err := r.reconcileSentinelStatefulSet(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Sentinel StatefulSet")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// Reconcile Redis Service
	if err := r.reconcileRedisService(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Redis Service")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// Reconcile Sentinel Service
	if err := r.reconcileSentinelService(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Sentinel Service")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// Reconcile ConfigMaps
	if err := r.reconcileConfigMaps(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMaps")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// Reconcile Persistent Volumes
	if err := r.reconcilePersistentVolumes(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Persistent Volumes")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// 更新角色标签
	if err := r.updateRedisRoleLabels(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to update Redis role labels")
		r.Recorder.Eventf(redisSentinel, corev1.EventTypeWarning, "LabelUpdateFailed",
			"Failed to update Redis role labels: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseError, err)
	}

	// 创建/更新主节点端点
	if err := r.reconcileRedisMasterEndpoints(ctx, redisSentinel); err != nil {
		logger.Error(err, "Failed to reconcile Redis master Endpoints")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status
	return ctrl.Result{RequeueAfter: 30 * time.Second}, r.updateStatus(ctx, redisSentinel, cachev1.RedisPhaseReady, nil)
}

func (r *RedisSentinelReconciler) reconcileRedisStatefulSet(ctx context.Context, rs *cachev1.RedisSentinel) error {
	logger := log.FromContext(ctx)
	name := rs.Name + "-redis"

	// Get replicas
	replicas := rs.Spec.RedisReplicas
	if replicas < 1 {
		replicas = 3
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "redis"),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name + "-headless",
			Replicas:    pointer.Int32(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForRedisSentinel(rs.Name, "redis"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForRedisSentinel(rs.Name, "redis"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           rs.Spec.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: RedisPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "redis-config",
									MountPath: "/redis-config",
								},
								{
									Name:      "redis-data",
									MountPath: "/data",
								},
							},
							Command: []string{
								"sh", "-c",
								"sh /redis-config/init.sh",
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "redis-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: rs.Name + "-redis-config",
									},
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
								corev1.ResourceStorage: rs.Spec.Storage.Size,
							},
						},
						StorageClassName: pointer.String("redis-storage"),
					},
				},
			},
		},
	}

	// Set controller reference
	if err := ctrl.SetControllerReference(rs, sts, r.Scheme); err != nil {
		return err
	}

	// Create or update StatefulSet
	foundSts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, foundSts)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Redis StatefulSet", "name", sts.Name)
		if err := r.Create(ctx, sts); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		logger.Info("Updating Redis StatefulSet", "name", sts.Name)
		sts.Spec.DeepCopyInto(&foundSts.Spec)
		if err := r.Update(ctx, foundSts); err != nil {
			return err
		}
	}

	return nil
}

func (r *RedisSentinelReconciler) reconcileSentinelStatefulSet(ctx context.Context, rs *cachev1.RedisSentinel) error {
	logger := log.FromContext(ctx)
	name := rs.Name + "-sentinel"

	// Get replicas
	replicas := rs.Spec.SentinelReplicas
	if replicas < 1 {
		replicas = 3
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "sentinel"),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name + "-headless",
			Replicas:    pointer.Int32(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForRedisSentinel(rs.Name, "sentinel"),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForRedisSentinel(rs.Name, "sentinel"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "sentinel",
							Image:           rs.Spec.SentinelImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "sentinel",
									ContainerPort: SentinelPort,
								},
							},
							Command: []string{"sh", "-c",
								"sh /sentinel-config/init.sh",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sentinel-config",
									MountPath: "/sentinel-config",
								},
								{
									Name:      "sentinel-config-dir",
									MountPath: "/tmp", // 可写目录
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sentinel-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-sentinel-config", rs.Name),
									},
								},
							},
						},
						{
							Name: "sentinel-config-dir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	// Set controller reference
	if err := ctrl.SetControllerReference(rs, sts, r.Scheme); err != nil {
		return err
	}

	// Create or update StatefulSet
	foundSts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, foundSts)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Sentinel StatefulSet", "name", sts.Name)
		if err := r.Create(ctx, sts); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		logger.Info("Updating Sentinel StatefulSet", "name", sts.Name)
		sts.Spec.DeepCopyInto(&foundSts.Spec)
		if err := r.Update(ctx, foundSts); err != nil {
			return err
		}
	}

	return nil
}

func (r *RedisSentinelReconciler) reconcileRedisService(ctx context.Context, rs *cachev1.RedisSentinel) error {
	// Headless Service for StatefulSet
	headlessSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-redis-headless",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "redis"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labelsForRedisSentinel(rs.Name, "redis"),
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       RedisPort,
					TargetPort: intstr.FromInt(int(RedisPort)),
				},
			},
		},
	}

	// NodePort Service for external access
	nodePortSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-redis",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "redis"),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labelsForRedisSentinel(rs.Name, "redis"),
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       RedisPort,
					TargetPort: intstr.FromInt(int(RedisPort)),
					NodePort:   rs.Spec.NodePort,
				},
			},
		},
	}

	// NodePort Master Service
	selector := labelsForRedisSentinel(rs.Name, "redis")
	selector["redis-role"] = "master"
	masterNodePortSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-redis-master",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "redis"),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: selector,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       MasterPort,
					TargetPort: intstr.FromInt(int(RedisPort)),
					NodePort:   rs.Spec.MasterNodePort,
				},
			},
		},
	}

	// Set controller references
	if err := ctrl.SetControllerReference(rs, headlessSvc, r.Scheme); err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(rs, nodePortSvc, r.Scheme); err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(rs, masterNodePortSvc, r.Scheme); err != nil {
		return err
	}

	// Create or update services
	if err := r.createOrUpdateService(ctx, headlessSvc); err != nil {
		return err
	}
	if err := r.createOrUpdateService(ctx, nodePortSvc); err != nil {
		return err
	}
	return r.createOrUpdateService(ctx, masterNodePortSvc)
}

func (r *RedisSentinelReconciler) reconcileSentinelService(ctx context.Context, rs *cachev1.RedisSentinel) error {
	// Headless Service for StatefulSet
	headlessSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-sentinel-headless",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "sentinel"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labelsForRedisSentinel(rs.Name, "sentinel"),
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       SentinelPort,
					TargetPort: intstr.FromInt(int(SentinelPort)),
				},
			},
		},
	}

	// NodePort Service for external access
	nodePortSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-sentinel",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "sentinel"),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labelsForRedisSentinel(rs.Name, "sentinel"),
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       SentinelPort,
					TargetPort: intstr.FromInt(int(SentinelPort)),
					NodePort:   rs.Spec.SentinelNodePort,
				},
			},
		},
	}

	// Set controller references
	if err := ctrl.SetControllerReference(rs, headlessSvc, r.Scheme); err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(rs, nodePortSvc, r.Scheme); err != nil {
		return err
	}

	// Create or update services
	if err := r.createOrUpdateService(ctx, headlessSvc); err != nil {
		return err
	}
	return r.createOrUpdateService(ctx, nodePortSvc)
}

func (r *RedisSentinelReconciler) reconcileConfigMaps(ctx context.Context, rs *cachev1.RedisSentinel) error {
	masterHost := fmt.Sprintf("%s-redis-0.%s-redis-headless.%s.svc.cluster.local", rs.Name, rs.Name, rs.Namespace)

	// Redis ConfigMap
	redisCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-redis-config",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "redis"),
		},
		Data: map[string]string{
			"redis-master.conf":  redisMasterConfig,
			"redis-replica.conf": redisReplicaConfig,
			"init.sh":            redisInitSh(masterHost),
		},
	}

	sentinelCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-sentinel-config",
			Namespace: rs.Namespace,
			Labels:    labelsForRedisSentinel(rs.Name, "sentinel"),
		},
		Data: map[string]string{
			"sentinel.conf": sentinelConfig(masterHost, rs.Spec.SentinelReplicas),
			"init.sh":       sentinelInitConfig,
		},
	}

	// Set controller references
	if err := ctrl.SetControllerReference(rs, redisCM, r.Scheme); err != nil {
		return err
	}
	if err := ctrl.SetControllerReference(rs, sentinelCM, r.Scheme); err != nil {
		return err
	}

	// Create or update ConfigMaps
	if err := r.createOrUpdateConfigMap(ctx, redisCM); err != nil {
		return err
	}
	return r.createOrUpdateConfigMap(ctx, sentinelCM)
}

func (r *RedisSentinelReconciler) reconcilePersistentVolumes(ctx context.Context, rs *cachev1.RedisSentinel) error {
	logger := log.FromContext(ctx)

	replicas := rs.Spec.RedisReplicas
	if replicas < 1 {
		replicas = 3
	}

	// Create PVs for each Redis instance
	for i := 0; i < int(replicas); i++ {
		pvName := fmt.Sprintf("%s-redis-pv-%d", rs.Name, i)
		pvPath := fmt.Sprintf("%s/%s/redis-%d", rs.Spec.Storage.HostPath, rs.Name, i)

		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvName,
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: rs.Spec.Storage.Size,
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				StorageClassName:              "redis-storage",
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: pvPath,
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "iguochan.io/redis-node",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{fmt.Sprintf("redis%d", i%3+1)},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create PV
		if err := r.Create(ctx, pv); err != nil {
			if !errors.IsAlreadyExists(err) {
				logger.Error(err, "Failed to create PV", "name", pv.Name)
				return err
			}
			logger.Info("PV already exists", "name", pv.Name)
		}
	}

	// Create PVs for each Sentinel instance (if needed)
	for i := 0; i < int(rs.Spec.SentinelReplicas); i++ {
		pvName := fmt.Sprintf("%s-sentinel-pv-%d", rs.Name, i)
		pvPath := fmt.Sprintf("%s/%s/sentinel-%d", rs.Spec.Storage.HostPath, rs.Name, i)

		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvName,
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				StorageClassName:              "redis-storage",
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: pvPath,
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "iguochan.io/redis-node",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{fmt.Sprintf("redis%d", i%3+1)},
									},
								},
							},
						},
					},
				},
			},
		}

		// Create PV
		if err := r.Create(ctx, pv); err != nil {
			if !errors.IsAlreadyExists(err) {
				logger.Error(err, "Failed to create PV", "name", pv.Name)
				return err
			}
			logger.Info("PV already exists", "name", pv.Name)
		}
	}

	return nil
}

func (r *RedisSentinelReconciler) updateStatus(ctx context.Context, rs *cachev1.RedisSentinel, phase cachev1.RedisPhase, err error) error {
	if err != nil && phase != cachev1.RedisPhaseReady {
		rs.Status.Phase = phase
		_ = r.Status().Update(ctx, rs)
		return fmt.Errorf("err: %+v or phase: %s", err, phase)
	}

	// 1. 确保所有 Sentinel Pods 就绪
	sentinelReady, sentinelErr := r.checkPodsReady(ctx, rs, "sentinel")
	if !sentinelReady {
		return sentinelErr
	}

	// 2. 确保所有 Redis Pods 就绪（主节点必须就绪）
	redisReady, redisErr := r.checkPodsReady(ctx, rs, "redis")
	if !redisReady {
		return redisErr
	}

	// 3. 确保 Sentinel 服务可用
	sentinelSvc, svcErr := r.validateService(ctx, rs, rs.Name+"-sentinel")
	if svcErr != nil {
		return svcErr
	}

	// 4. 确保 Redis 服务可用（指向主节点）
	redisSvc, svcErr := r.validateService(ctx, rs, rs.Name+"-redis")
	if svcErr != nil {
		return svcErr
	}

	// 5. 更新状态
	rs.Status.SentinelEndpoint = fmt.Sprintf("%s:%d", sentinelSvc.Spec.ClusterIP, sentinelSvc.Spec.Ports[0].Port)
	rs.Status.Endpoint = fmt.Sprintf("%s:%d", redisSvc.Spec.ClusterIP, redisSvc.Spec.Ports[0].Port)
	rs.Status.Phase = cachev1.RedisPhaseReady

	return r.Status().Update(ctx, rs)
}

// 辅助函数：检查特定角色的 Pods 状态
func (r *RedisSentinelReconciler) checkPodsReady(ctx context.Context, rs *cachev1.RedisSentinel, role string) (bool, error) {
	podList := &corev1.PodList{}
	labels := client.MatchingLabels{
		"app":       "redis-sentinel",
		"name":      rs.Name,
		"component": role, // "redis" 或 "sentinel"
	}

	if err := r.List(ctx, podList, labels); err != nil {
		r.Recorder.Eventf(rs, corev1.EventTypeWarning, RecordReasonFailed,
			"list %s pods failed: %s", role, err.Error())
		return false, err
	}

	if len(podList.Items) == 0 {
		msg := fmt.Sprintf("no %s pods available", role)
		r.Recorder.Event(rs, corev1.EventTypeNormal, RecordReasonWaiting, msg)
		rs.Status.Phase = cachev1.RedisPhasePending
		return false, r.Status().Update(ctx, rs)
	}

	allReady := true
	for _, pod := range podList.Items {
		if !isPodReady(pod) {
			allReady = false
			break
		}
	}

	if !allReady {
		msg := fmt.Sprintf("not all %s pods are ready", role)
		r.Recorder.Event(rs, corev1.EventTypeNormal, RecordReasonWaiting, msg)
		rs.Status.Phase = cachev1.RedisPhasePending
		return false, r.Status().Update(ctx, rs)
	}

	return true, nil
}

// 辅助函数：检查 Pod 就绪状态
func isPodReady(pod corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// 辅助函数：验证服务可用性
func (r *RedisSentinelReconciler) validateService(ctx context.Context, rs *cachev1.RedisSentinel, svcName string) (*corev1.Service, error) {
	svc := &corev1.Service{}
	key := types.NamespacedName{Namespace: rs.Namespace, Name: svcName}

	// 获取 Service 对象
	if err := r.Get(ctx, key, svc); err != nil {
		r.Recorder.Eventf(rs, corev1.EventTypeWarning, RecordReasonFailed,
			"get %s service failed: %s", svcName, err.Error())
		rs.Status.Phase = cachev1.RedisPhaseError
		return nil, r.Status().Update(ctx, rs)
	}

	// 验证对应的 Endpoints
	endpoints := &corev1.Endpoints{}
	if err := r.Get(ctx, key, endpoints); err != nil {
		r.Recorder.Eventf(rs, corev1.EventTypeWarning, RecordReasonFailed,
			"get %s endpoints failed: %s", svcName, err.Error())
		rs.Status.Phase = cachev1.RedisPhaseError
		return nil, r.Status().Update(ctx, rs)
	}

	// 检查可用终端
	if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
		r.Recorder.Eventf(rs, corev1.EventTypeWarning, RecordReasonFailed,
			"%s service has no endpoints", svcName)
		rs.Status.Phase = cachev1.RedisPhaseError
		return nil, r.Status().Update(ctx, rs)
	}

	return svc, nil
}

// Helper functions
func labelsForRedisSentinel(name, role string) map[string]string {
	return map[string]string{
		"app":       "redis-sentinel",
		"name":      name,
		"component": role,
	}
}

func (r *RedisSentinelReconciler) createOrUpdateService(ctx context.Context, svc *corev1.Service) error {
	foundSvc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, foundSvc)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, svc)
	} else if err != nil {
		return err
	}

	// Preserve existing NodePort if not specified
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		for i, p := range svc.Spec.Ports {
			foundSvc.Spec.Ports[i].Port = p.Port
			foundSvc.Spec.Ports[i].TargetPort = p.TargetPort
			foundSvc.Spec.Ports[i].NodePort = p.NodePort
		}
	}

	return r.Update(ctx, foundSvc)
}

func (r *RedisSentinelReconciler) createOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	foundCM := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, foundCM)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, cm)
	} else if err != nil {
		return err
	}

	foundCM.Data = cm.Data
	return r.Update(ctx, foundCM)
}

func (r *RedisSentinelReconciler) updateRedisRoleLabels(ctx context.Context, rs *cachev1.RedisSentinel) error {
	// 获取所有 Redis Pod
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		"app":       "redis-sentinel",
		"name":      rs.Name,
		"component": "redis",
	}); err != nil {
		return err
	}

	// 获取主节点地址
	var ip string
	var err error
	sentinelPods := &corev1.PodList{}
	if err = r.List(ctx, sentinelPods, client.MatchingLabels{
		"app":       "redis-sentinel",
		"name":      rs.Name,
		"component": "sentinel",
	}); err != nil || len(sentinelPods.Items) == 0 {
		return fmt.Errorf("list sentinel pods err: %+v or len(sentinelPods.Items) == 0", err)
	}

	if ip, _, err = r.getSentinelMasterAddr(ctx, &sentinelPods.Items[0]); err != nil {
		return err
	}

	// 更新 Pod 标签
	for _, pod := range podList.Items {
		newRole := "slave"
		if pod.Status.PodIP == ip || strings.Contains(ip, pod.Spec.Hostname) {
			newRole = "master"
		}

		if pod.Labels["redis-role"] != newRole {
			patch := client.MergeFrom(pod.DeepCopy())
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			pod.Labels["redis-role"] = newRole
			if err := r.Patch(ctx, &pod, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RedisSentinelReconciler) reconcileRedisMasterEndpoints(ctx context.Context, rs *cachev1.RedisSentinel) error {
	// 获取主节点 Pod
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		"app":        "redis-sentinel",
		"name":       rs.Name,
		"component":  "redis",
		"redis-role": "master",
	}); err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		return nil // 没有主节点
	}

	masterPod := podList.Items[0]
	if masterPod.Status.PodIP == "" {
		return fmt.Errorf("reconcileRedisMasterEndpoints: masterPod.Status.PodIP is empty")
	}

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rs.Name + "-redis-master",
			Namespace: rs.Namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: masterPod.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      masterPod.Name,
							Namespace: masterPod.Namespace,
						},
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Port: RedisPort,
					},
				},
			},
		},
	}

	// 设置控制器引用
	if err := ctrl.SetControllerReference(rs, endpoints, r.Scheme); err != nil {
		return err
	}

	// 创建或更新端点
	found := &corev1.Endpoints{}
	err := r.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, endpoints)
	} else if err != nil {
		return err
	}

	// 比较并更新
	needsUpdate := false
	if len(found.Subsets) == 0 {
		needsUpdate = true
	} else if len(found.Subsets[0].Addresses) == 0 ||
		found.Subsets[0].Addresses[0].IP != masterPod.Status.PodIP {
		needsUpdate = true
	}

	if needsUpdate {
		found.Subsets = endpoints.Subsets
		return r.Update(ctx, found)
	}

	return nil
}

func (r *RedisSentinelReconciler) getSentinelMasterAddr(ctx context.Context, sentinelPod *corev1.Pod) (string, string, error) {
	logger := log.FromContext(ctx)

	sentinelAddr := fmt.Sprintf("%s:%d", sentinelPod.Status.PodIP, SentinelPort)
	sentinelClient := redis.NewSentinelClient(&redis.Options{
		Addr:     sentinelAddr,
		Password: "", // 如果有密码需要添加
		DB:       0,
	})

	// 添加故障转移检测
	var masterIP, masterPort string
	var lastErr error

	// 最多重试 5 次，每次间隔 2 秒
	for i := 0; i < 5; i++ {
		result, err := sentinelClient.GetMasterAddrByName("mymaster").Result()
		if err == nil && len(result) >= 2 {
			masterIP = result[0]
			masterPort = result[1]

			// 验证主节点是否实际存在
			if r.isPodAlive(ctx, masterIP) {
				logger.Info(fmt.Sprintf("getSentinelMasterAddr: %s, %s", masterIP, masterPort))
				return masterIP, masterPort, nil
			}
			logger.Info("Master IP reported but pod not alive", "ip", masterIP)
		} else if err != nil {
			lastErr = err
		}

		// 等待 2 秒后重试
		time.Sleep(2 * time.Second)
	}

	return "", "", fmt.Errorf("failed to get valid master address after 5 attempts: %v", lastErr)
}

// 检查 Pod 是否实际存在
func (r *RedisSentinelReconciler) isPodAlive(ctx context.Context, ip string) bool {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods); err != nil {
		return false
	}

	for _, pod := range pods.Items {
		if pod.Status.PodIP == ip || strings.Contains(ip, pod.Spec.Hostname) {
			return pod.DeletionTimestamp == nil
		}
	}
	return false
}

func (r *RedisSentinelReconciler) checkPodDeletion(ctx context.Context, rs *cachev1.RedisSentinel) bool {
	// 获取所有 Redis Pod
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingLabels{
		"app":       "redis-sentinel",
		"name":      rs.Name,
		"component": "redis",
	}); err != nil {
		return false
	}

	// 检查是否有 Pod 正在删除中
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisSentinelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1.RedisSentinel{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Endpoints{}).
		Complete(r)
}
