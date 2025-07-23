package controller

import "fmt"

var (
	redisMasterConfig = fmt.Sprintf(`
# 主节点基础配置
bind 0.0.0.0
port %d
appendonly yes
cluster-enabled no
protected-mode no
`, RedisPort)

	redisReplicaConfig = fmt.Sprintf(`
# 从节点基础配置
bind 0.0.0.0
port %d
appendonly yes
replica-read-only yes
protected-mode no
`, RedisPort)

	sentinelInitConfig = fmt.Sprintf(`
# 启动脚本
# 拷贝脚本
cp /sentinel-config/sentinel.conf /data/sentinel.conf
chmod 644 /data/sentinel.conf

# 记录配置
echo "Sentinel Configuration:"
cat /data/sentinel.conf

# 启动Sentinel
echo "==== Starting Sentinel ===="
exec redis-sentinel /data/sentinel.conf
`)
)

func redisInitSh(masterHost string) string {
	return fmt.Sprintf(`
# 从节点基础配置
#!/bin/sh
set -ex
POD_INDEX=${HOSTNAME##*-}
echo "Current pod index: $POD_INDEX"

# 索引0为主节点，其他为从节点
if [ "$POD_INDEX" = "0" ]; then
cp /redis-config/redis-master.conf /data/redis.conf
echo "Running as master"
else
cp /redis-config/redis-replica.conf /data/redis.conf
echo "# Slave of master: masterHost" >> /data/redis.conf
echo "replicaof %s %d" >> /data/redis.conf
echo "Running as slave"
fi

# 记录配置
echo "Sentinel Configuration:"
cat /data/redis.conf

# 启动Sentinel
echo "==== Starting Redis ===="
exec redis-server /data/redis.conf
`, masterHost, RedisPort)
}

func sentinelConfig(masterHost string, sentinelReplica int32) string {
	return fmt.Sprintf(`
# 基础配置
port %d
sentinel resolve-hostnames yes
sentinel announce-hostnames yes
sentinel monitor mymaster %s %d %d
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 10000
sentinel parallel-syncs mymaster 1
protected-mode no
`, SentinelPort, masterHost, RedisPort, sentinelReplica/2+1)

}
