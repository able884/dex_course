# RocketMQ Docker 部署指南

## 快速启动

### 1. 启动 RocketMQ 集群

```bash
# 在项目根目录执行
docker-compose -f docker-compose-rocketmq.yml up -d
```

### 2. 检查服务状态

```bash
# 查看容器状态
docker-compose -f docker-compose-rocketmq.yml ps

# 查看 NameServer 日志
docker logs rocketmq-nameserver

# 查看 Broker 日志
docker logs rocketmq-broker
```

### 3. 创建项目所需的主题

等待服务完全启动后（约 30 秒），创建项目所需的主题：

```bash
# 方式一：使用提供的脚本
bash rocketmq/create-topics.sh

# 方式二：手动创建主题
docker exec -it rocketmq-broker sh mqadmin updateTopic -n rocketmq-nameserver:9876 -t solana-blocks -c DefaultCluster -r 8 -w 8
docker exec -it rocketmq-broker sh mqadmin updateTopic -n rocketmq-nameserver:9876 -t solana-blocks-retry -c DefaultCluster -r 8 -w 8
docker exec -it rocketmq-broker sh mqadmin updateTopic -n rocketmq-nameserver:9876 -t pump-migration -c DefaultCluster -r 8 -w 8
```

### 4. 创建消费者组

```bash
docker exec -it rocketmq-broker sh mqadmin updateSubGroup -n rocketmq-nameserver:9876 -c DefaultCluster -g block-consumer-group
docker exec -it rocketmq-broker sh mqadmin updateSubGroup -n rocketmq-nameserver:9876 -c DefaultCluster -g block-consumer-group-dlq-handler
```

### 5. 访问管理控制台

打开浏览器访问：`http://localhost:8090`

默认无需登录，可以查看：
- Topic 列表
- Consumer 组状态
- 消息堆积情况
- 集群状态

## 常用命令

### 查看主题列表

```bash
docker exec -it rocketmq-broker sh mqadmin topicList -n rocketmq-nameserver:9876
```

### 查看主题详情

```bash
docker exec -it rocketmq-broker sh mqadmin topicStatus -n rocketmq-nameserver:9876 -t solana-blocks
```

### 查看消费者组

```bash
docker exec -it rocketmq-broker sh mqadmin consumerProgress -n rocketmq-nameserver:9876 -g block-consumer-group
```

### 查看 DLQ 消息

```bash
# 查看死信队列消息
docker exec -it rocketmq-broker sh mqadmin queryMsgById -n rocketmq-nameserver:9876 -t %DLQ%block-consumer-group -i <msgId>
```

### 清理主题数据

```bash
# 删除主题
docker exec -it rocketmq-broker sh mqadmin deleteTopic -n rocketmq-nameserver:9876 -c DefaultCluster -t solana-blocks

# 重新创建
docker exec -it rocketmq-broker sh mqadmin updateTopic -n rocketmq-nameserver:9876 -t solana-blocks -c DefaultCluster -r 8 -w 8
```

## 停止和清理

### 停止服务

```bash
docker-compose -f docker-compose-rocketmq.yml stop
```

### 重启服务

```bash
docker-compose -f docker-compose-rocketmq.yml restart
```

### 完全清理（包括数据）

```bash
# 停止并删除容器
docker-compose -f docker-compose-rocketmq.yml down

# 清理数据卷（谨慎操作！）
rm -rf rocketmq/broker/logs rocketmq/broker/store rocketmq/nameserver/logs
```

## 配置说明

### Broker 配置文件：`rocketmq/broker.conf`

关键配置项：
- `maxMessageSize = 16777216` - 最大消息 16MB（匹配项目配置）
- `autoCreateTopicEnable = true` - 自动创建主题
- `consumerTimeoutMillisWhenSuspend = 900000` - 消费超时 15 分钟

### Docker Compose 配置

- **NameServer 端口**: 9876
- **Broker 端口**:
  - 10909 (fastRemotingServer)
  - 10911 (listenPort)
  - 10912 (haListenPort)
- **Console 端口**: 8090

## 项目配置

确保 `consumer/etc/consumer.yaml` 中的 RocketMQ 配置正确：

```yaml
RocketMQ:
  NameServers:
    - "127.0.0.1:9876"
  MaxMessageSize: 16777216  # 16MB
  Topics:
    Blocks: "solana-blocks"
    Retry: "solana-blocks-retry"
    Migration: "pump-migration"
  Producer:
    SendTimeout: 10s
    RetryTimes: 2
    CompressLevel: 6
  Consumer:
    GroupName: "block-consumer-group"
    ConsumeFromWhere: "CONSUME_FROM_LAST_OFFSET"
    MessageModel: "CLUSTERING"
    MaxReconsumeTimes: 3
    ConsumeTimeout: 15m
```

## 性能监控

### 查看消息堆积

```bash
# 方式一：命令行
docker exec -it rocketmq-broker sh mqadmin consumerProgress -n rocketmq-nameserver:9876 -g block-consumer-group

# 方式二：Web 控制台
# 访问 http://localhost:8090 -> Consumer 页面
```

### 查看 Broker 统计

```bash
docker exec -it rocketmq-broker sh mqadmin brokerStats -n rocketmq-nameserver:9876 -b rocketmq-broker:10911
```

## 故障排查

### 1. 连接失败

检查 NameServer 是否启动：
```bash
docker logs rocketmq-nameserver
telnet localhost 9876
```

### 2. 消息发送失败

检查 Broker 是否注册：
```bash
docker exec -it rocketmq-broker sh mqadmin clusterList -n rocketmq-nameserver:9876
```

### 3. 消费堆积严重

```bash
# 增加消费者并发数（修改 consumer.yaml）
Consumer:
  Concurrency: 20  # 从 10 增加到 20

# 或者临时增加消费者实例数量
```

### 4. 查看详细日志

```bash
# Broker 日志
docker logs -f rocketmq-broker

# 进入容器查看详细日志
docker exec -it rocketmq-broker sh
cd /home/rocketmq/logs
tail -f rocketmqlogs/broker.log
```

## 生产环境建议

1. **持久化存储**：使用 Docker volume 或外部存储
2. **资源限制**：根据消息量调整内存（建议 4GB+）
3. **高可用部署**：配置多个 Broker（主从模式）
4. **监控告警**：集成 Prometheus + Grafana
5. **日志管理**：配置日志轮转和归档

## Windows 特殊说明

如果在 Windows 上运行，需要调整路径：

```yaml
# docker-compose-rocketmq.yml 中的 volumes
volumes:
  - ./rocketmq/broker.conf:/home/rocketmq/broker.conf
  # Windows 路径示例：
  # - C:/Users/YourName/project/rocketmq/broker.conf:/home/rocketmq/broker.conf
```

或者使用 WSL2 运行 Docker。
