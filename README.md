# <center> Redis Operator[🔗](https://juejin.cn/post/7515487645004152871)

## 0. 简介

Redis Operator 实现了单机版的Operator。就目前而言，其主要用于Operator自学，可以参考[8. Redis Operator (1) —— 单机部署](https://juejin.cn/post/7515487645004152871)。
更多功能或者成熟的Redis Operator方案请参考其他工程，比如[Redis Operator](https://operatorhub.io/operator/redis-operator)。

## 1. Kind 集群

### 工具版本
*   `go`: v1.20.2
*   `kubebuilder`: v3.10.0
*   `kind`: v0.20.0
*   `kustomize`: v5.0.0
*   `controller-gen`: v0.11.3
*   `setup-envtest`: release-0.17

### kind 配置
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 31000
    hostPort: 6379
    protocol: TCP
  extraMounts:
    # 主机目录映射到节点容器
    - hostPath: /Users/xxx/workspace/k8s/kind/data
      containerPath: /data
- role: worker
  extraMounts:
  # 主机目录映射到节点容器
  - hostPath: /Users/xxx/workspace/k8s/kind/data
    containerPath: /data
```

命令：`kind create cluster --name single --config single.yaml`创建集群；创建别名`alias ks='kubectl --context kind-single'`。


### 代码配置

#### 下载代码
```
git clone git@github.com:IguoChan/redis-operator.git
cd redis-operator
```

#### cert-manager
```
ks apply -f https://github.com/jetstack/cert-manager/releases/download/v1.8.0/cert-manager.yaml
```


#### redis image
```
kind load docker-image redis:7.0 redis:7.0 --name single
```

#### controller-image
```
make docker-build
kind load docker-image redis-controller:latest redis-controller:latest --name single
make deploy
```

#### redis
```
ks apply -f config/samples/cache_v1_redis.yaml
```

#### 验证
```
redis-cli -h 127.0.0.1 -p 6379
127.0.0.1:6379> get key1
(nil)
127.0.0.1:6379> set key1 hello
OK
127.0.0.1:6379> get key1
"hello"
```