#!/bin/bash

# RocketMQ 健康检查脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

NAMESERVER="rocketmq-nameserver:9876"
BROKER_CONTAINER="rocketmq-broker"

echo -e "${BLUE}=== RocketMQ 健康检查 ===${NC}\n"

# 1. 检查容器状态
echo -e "${YELLOW}[1/6] 检查 Docker 容器状态...${NC}"
if docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "rocketmq" | grep -q "Up"; then
    echo -e "${GREEN}✓ 容器运行正常${NC}"
    docker ps --format "table {{.Names}}\t{{.Status}}" | grep "rocketmq"
else
    echo -e "${RED}✗ 容器未运行或异常${NC}"
    exit 1
fi
echo ""

# 2. 检查 NameServer
echo -e "${YELLOW}[2/6] 检查 NameServer 连接...${NC}"
if docker exec -it $BROKER_CONTAINER sh mqadmin clusterList -n $NAMESERVER 2>&1 | grep -q "broker-a"; then
    echo -e "${GREEN}✓ NameServer 连接成功${NC}"
else
    echo -e "${RED}✗ NameServer 连接失败${NC}"
    exit 1
fi
echo ""

# 3. 检查主题是否存在
echo -e "${YELLOW}[3/6] 检查项目主题...${NC}"
REQUIRED_TOPICS=("solana-blocks" "solana-blocks-retry" "pump-migration")
TOPIC_LIST=$(docker exec -it $BROKER_CONTAINER sh mqadmin topicList -n $NAMESERVER 2>/dev/null)

for topic in "${REQUIRED_TOPICS[@]}"; do
    if echo "$TOPIC_LIST" | grep -q "$topic"; then
        echo -e "${GREEN}✓ 主题存在: $topic${NC}"
    else
        echo -e "${RED}✗ 主题缺失: $topic${NC}"
        echo "  运行创建脚本: ./create-topics.sh"
    fi
done
echo ""

# 4. 检查消费者组
echo -e "${YELLOW}[4/6] 检查消费者组...${NC}"
REQUIRED_GROUPS=("block-consumer-group" "block-consumer-group-dlq-handler")
GROUP_LIST=$(docker exec -it $BROKER_CONTAINER sh mqadmin consumerProgress -n $NAMESERVER 2>/dev/null || echo "")

for group in "${REQUIRED_GROUPS[@]}"; do
    if echo "$GROUP_LIST" | grep -q "$group" || \
       docker exec -it $BROKER_CONTAINER sh mqadmin updateSubGroup -n $NAMESERVER -c DefaultCluster -g "$group" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ 消费者组存在: $group${NC}"
    else
        echo -e "${YELLOW}⚠ 消费者组可能不存在: $group${NC}"
    fi
done
echo ""

# 5. 检查 Broker 配置
echo -e "${YELLOW}[5/6] 检查 Broker 配置...${NC}"
BROKER_CONFIG=$(docker exec -it $BROKER_CONTAINER sh mqadmin getBrokerConfig -n $NAMESERVER -b rocketmq-broker:10911 2>/dev/null || echo "")

# 检查最大消息大小
if echo "$BROKER_CONFIG" | grep -q "maxMessageSize.*16777216"; then
    echo -e "${GREEN}✓ 最大消息大小: 16MB${NC}"
else
    echo -e "${YELLOW}⚠ 最大消息大小配置可能不正确${NC}"
fi

# 检查自动创建主题
if echo "$BROKER_CONFIG" | grep -q "autoCreateTopicEnable.*true"; then
    echo -e "${GREEN}✓ 自动创建主题: 已启用${NC}"
else
    echo -e "${YELLOW}⚠ 自动创建主题: 未启用${NC}"
fi
echo ""

# 6. 检查管理控制台
echo -e "${YELLOW}[6/6] 检查管理控制台...${NC}"
if curl -s http://localhost:8090 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 控制台可访问: http://localhost:8090${NC}"
else
    echo -e "${YELLOW}⚠ 控制台不可访问，请检查容器: rocketmq-console${NC}"
fi
echo ""

# 总结
echo -e "${BLUE}=== 健康检查完成 ===${NC}\n"
echo -e "${GREEN}系统信息：${NC}"
echo "  - NameServer: 127.0.0.1:9876"
echo "  - 管理控制台: http://localhost:8090"
echo "  - Broker: rocketmq-broker:10911"
echo ""

# 显示主题详情
echo -e "${BLUE}=== 主题详情 ===${NC}"
for topic in "${REQUIRED_TOPICS[@]}"; do
    echo -e "\n${YELLOW}主题: $topic${NC}"
    docker exec -it $BROKER_CONTAINER sh mqadmin topicStatus -n $NAMESERVER -t "$topic" 2>/dev/null | head -n 10 || echo "无法获取详情"
done

echo ""
echo -e "${GREEN}✓ 所有检查完成！${NC}"
echo "如需帮助，请查看: rocketmq/README.md"
