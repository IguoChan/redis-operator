# <center> Redis Operator[🔗](https://juejin.cn/post/7515487645004152871)

## 0. 简介

Redis Operator 实现了单机版的Operator。就目前而言，其主要用于Operator自学，可以参考[8. Redis Operator (1) —— 单机部署](https://juejin.cn/post/7515487645004152871)。
更多功能或者成熟的Redis Operator方案请参考其他工程，比如[Redis Operator](https://operatorhub.io/operator/redis-operator)。

## 1. Redis Standalone
### Kind 集群

#### 工具版本
*   `go`: v1.20.2
*   `kubebuilder`: v3.10.0
*   `kind`: v0.20.0
*   `kustomize`: v5.0.0
*   `controller-gen`: v0.11.3
*   `setup-envtest`: release-0.17

#### kind 配置
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

### 验证
```
redis-cli -h 127.0.0.1 -p 6379
127.0.0.1:6379> get key1
(nil)
127.0.0.1:6379> set key1 hello
OK
127.0.0.1:6379> get key1
"hello"
```

## 2. Redis Sentinel

### Kind 集群

#### kind 配置
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  # port forward 80 on the host to 80 on this node
  extraPortMappings:
  - containerPort: 30950
    hostPort: 80
    # optional: set the bind address on the host
    # 0.0.0.0 is the current default
    listenAddress: "127.0.0.1"
    # optional: set the protocol to one of TCP, UDP, SCTP.
    # TCP is the default
    protocol: TCP
  - containerPort: 30999
    hostPort: 6378
    protocol: TCP
  - containerPort: 31000
    hostPort: 6379
    protocol: TCP
  - containerPort: 31001
    hostPort: 26379
    protocol: TCP
- role: worker
  extraMounts:
  # 主机目录映射到节点容器
  - hostPath: /Users/chenyiguo/workspace/k8s/kind/multi_data/worker1
    containerPath: /data
  labels:
    iguochan.io/redis-node: redis1
- role: worker
  extraMounts:
  # 主机目录映射到节点容器
  - hostPath: /Users/chenyiguo/workspace/k8s/kind/multi_data/worker2
    containerPath: /data
  labels:
    iguochan.io/redis-node: redis2
- role: worker
  extraMounts:
  # 主机目录映射到节点容器
    - hostPath: /Users/chenyiguo/workspace/k8s/kind/multi_data/worker3
      containerPath: /data
  labels:
    iguochan.io/redis-node: redis3
```

命令：`kind create cluster --name multi --config multi.yaml`创建集群；创建别名`alias km='kubectl --context kind-multi'`。

### 代码配置

#### 下载代码
```
git clone git@github.com:IguoChan/redis-operator.git
cd redis-operator
```

#### cert-manager
```
km apply -f https://github.com/jetstack/cert-manager/releases/download/v1.8.0/cert-manager.yaml
```


#### redis image
```
kind load docker-image redis:7.0 redis:7.0 --name multi
```

#### controller-image
```
make docker-build
kind load docker-image redis-controller:latest redis-controller:latest --name multi
make deploy
```
> 注意：`make`命令之前，kubectl需要指定是multi环境的集群。

#### redis
```
km apply -f config/samples/cache_v1_redissentinel.yaml
```

### 验证
通过一系列命令将CRD发布之后，我们开始验证。首先我们验证基本流程：

#### 主节点端口读写
``` bash
$ redis-cli -h 127.0.0.1 -p 6378
127.0.0.1:6378> get key2
(nil)
127.0.0.1:6378> set key2 hello
OK
```

#### 从节点端口读
``` bash
$ redis-cli -h 127.0.0.1 -p 6379
127.0.0.1:6379> get key2
"hello"
127.0.0.1:6379> set key2 hello1
(error) READONLY You can't write against a read only replica.
```
可以发现，从节点只能读不能写；但是这也不是一定的，因为很有可能长连接连接到的是主节点。

##### sentinel端口
``` bash
$ redis-cli -h 127.0.0.1 -p 26379
127.0.0.1:26379> SENTINEL master mymaster # 验证master
 1) "name"
 2) "mymaster"
 3) "ip"
 4) "redissentinel-sample-redis-0.redissentinel-sample-redis-headless.default.svc.cluster.local"
 5) "port"
 6) "6379"
 7) "runid"
 8) "7792152f59bc4716a8d88a76cd39ed19c2bc0c92"
 9) "flags"
10) "master"
11) "link-pending-commands"
12) "0"
13) "link-refcount"
14) "1"
15) "last-ping-sent"
16) "0"
17) "last-ok-ping-reply"
18) "415"
19) "last-ping-reply"
20) "415"
21) "down-after-milliseconds"
22) "5000"
23) "info-refresh"
24) "7391"
25) "role-reported"
26) "master"
27) "role-reported-time"
28) "25018208"
29) "config-epoch"
30) "0"
31) "num-slaves"
32) "2"
33) "num-other-sentinels"
34) "2"
35) "quorum"
36) "2"
37) "failover-timeout"
38) "10000"
39) "parallel-syncs"
40) "1"
127.0.0.1:26379> SENTINEL slaves mymaster # 验证slave
1)  1) "name"
    2) "10.244.1.5:6379"
    3) "ip"
    4) "10.244.1.5"
    5) "port"
    6) "6379"
    7) "runid"
    8) "099f9411d941e3bfe6888870afd260e9b5eea60e"
    9) "flags"
   10) "slave"
   11) "link-pending-commands"
   12) "0"
   13) "link-refcount"
   14) "1"
   15) "last-ping-sent"
   16) "0"
   17) "last-ok-ping-reply"
   18) "247"
   19) "last-ping-reply"
   20) "247"
   21) "down-after-milliseconds"
   22) "5000"
   23) "info-refresh"
   24) "7115"
   25) "role-reported"
   26) "slave"
   27) "role-reported-time"
   28) "25029569"
   29) "master-link-down-time"
   30) "0"
   31) "master-link-status"
   32) "ok"
   33) "master-host"
   34) "redissentinel-sample-redis-0.redissentinel-sample-redis-headless.default.svc.cluster.local"
   35) "master-port"
   36) "6379"
   37) "slave-priority"
   38) "100"
   39) "slave-repl-offset"
   40) "3549038"
   41) "replica-announced"
   42) "1"
2)  1) "name"
    2) "10.244.3.2:6379"
    3) "ip"
    4) "10.244.3.2"
    5) "port"
    6) "6379"
    7) "runid"
    8) "ae038757a97446ccc7325812d929b7c1e7a3fa0f"
    9) "flags"
   10) "slave"
   11) "link-pending-commands"
   12) "0"
   13) "link-refcount"
   14) "1"
   15) "last-ping-sent"
   16) "0"
   17) "last-ok-ping-reply"
   18) "247"
   19) "last-ping-reply"
   20) "247"
   21) "down-after-milliseconds"
   22) "5000"
   23) "info-refresh"
   24) "7241"
   25) "role-reported"
   26) "slave"
   27) "role-reported-time"
   28) "25029572"
   29) "master-link-down-time"
   30) "0"
   31) "master-link-status"
   32) "ok"
   33) "master-host"
   34) "redissentinel-sample-redis-0.redissentinel-sample-redis-headless.default.svc.cluster.local"
   35) "master-port"
   36) "6379"
   37) "slave-priority"
   38) "100"
   39) "slave-repl-offset"
   40) "3549038"
   41) "replica-announced"
   42) "1"
127.0.0.1:26379> SENTINEL get-master-addr-by-name mymaster
1) "redissentinel-sample-redis-0.redissentinel-sample-redis-headless.default.svc.cluster.local"
2) "6379"
```

##### failover验证
我们发现此时的主节点是`redissentinel-sample-redis-0`，这时候我们删了这个节点：
``` bash
$ k get pod --show-labels
NAME                              READY   STATUS    RESTARTS        AGE   LABELS
redissentinel-sample-redis-0      1/1     Running   2 (7h10m ago)   19d   app=redis-sentinel,component=redis,controller-revision-hash=redissentinel-sample-redis-9c894dbc9,name=redissentinel-sample,redis-role=master,statefulset.kubernetes.io/pod-name=redissentinel-sample-redis-0
redissentinel-sample-redis-1      1/1     Running   2 (7h10m ago)   19d   app=redis-sentinel,component=redis,controller-revision-hash=redissentinel-sample-redis-9c894dbc9,name=redissentinel-sample,redis-role=slave,statefulset.kubernetes.io/pod-name=redissentinel-sample-redis-1
redissentinel-sample-redis-2      1/1     Running   2 (7h10m ago)   19d   app=redis-sentinel,component=redis,controller-revision-hash=redissentinel-sample-redis-9c894dbc9,name=redissentinel-sample,redis-role=slave,statefulset.kubernetes.io/pod-name=redissentinel-sample-redis-2
$ k delete pod redissentinel-sample-redis-0
pod "redissentinel-sample-redis-0" deleted
```
此时我们回到主节点端口：
``` bash
127.0.0.1:6378> get key2
"hello"
127.0.0.1:6378> set key2 hello1
OK
127.0.0.1:6378> get key2
"hello1"
```
可以看到，主节点端口依然可以进行读写操作，我们再去看从节点端口：
``` bash
127.0.0.1:6379> get key2
"hello1"
127.0.0.1:6379> set key2 hello
(error) READONLY You can't write against a read only replica.
```
最后再去sentinel端口验证一下此时的主节点：
``` bash
127.0.0.1:26379> SENTINEL master mymaster
 ...
 3) "ip"
 4) "10.244.1.5"
 ...
```
而这个节点是节点3：
``` bash
$ k get pod -o wide
NAME                              READY   STATUS    RESTARTS        AGE     IP           NODE            NOMINATED NODE   READINESS GATES
redissentinel-sample-redis-0      1/1     Running   0               3m54s   10.244.2.6   multi-worker    <none>           <none>
redissentinel-sample-redis-1      1/1     Running   2 (7h21m ago)   19d     10.244.3.2   multi-worker2   <none>           <none>
redissentinel-sample-redis-2      1/1     Running   2 (7h21m ago)   19d     10.244.1.5   multi-worker3   <none>           <none>
```

但是这个方案还是有**很大问题**的，我在多次尝试后会发现：

1. 后续`redis-cli`需要重连，因为这些链接是TCP的长连接；
2. 如果发生了故障转移，可能需要一点时间才能将这个role转移过来，这点应该可以通过更优雅的代码实现，但是这里是做一个demo，我就不深究了，本质上是为了学习Operator的实现。