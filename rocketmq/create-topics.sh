#!/bin/bash

# RocketMQ 主题创建脚本
# 用于自动创建项目所需的主题和消费者组

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置
NAMESERVER="rocketmq-nameserver:9876"
CLUSTER="DefaultCluster"
BROKER_CONTAINER="rocketmq-broker"

# 主题配置（主题名, 读队列数, 写队列数）
declare -A TOPICS=(
    ["solana-blocks"]="8 8"
    ["solana-blocks-retry"]="8 8"
    ["pump-migration"]="4 4"
    ["%DLQ%block-consumer-group"]="1 1"
)

# 消费者组配置
CONSUMER_GROUPS=(
    "block-consumer-group"
    "block-consumer-group-dlq-handler"
)

echo -e "${GREEN}=== RocketMQ 主题和消费者组创建脚本 ===${NC}\n"

# 检查 Docker 容器是否运行
echo -e "${YELLOW}检查 RocketMQ 容器状态...${NC}"
if ! docker ps | grep -q "$BROKER_CONTAINER"; then
    echo -e "${RED}错误: RocketMQ Broker 容器未运行${NC}"
    echo "请先启动 RocketMQ: docker-compose -f docker-compose-rocketmq.yml up -d"
    exit 1
fi

# 等待 Broker 完全启动
echo -e "${YELLOW}等待 Broker 启动完成...${NC}"
sleep 5

# 检查 NameServer 连接
echo -e "${YELLOW}检查 NameServer 连接...${NC}"
if docker exec -it $BROKER_CONTAINER sh mqadmin clusterList -n $NAMESERVER > /dev/null 2>&1; then
    echo -e "${GREEN}✓ NameServer 连接成功${NC}\n"
else
    echo -e "${RED}✗ 无法连接到 NameServer${NC}"
    exit 1
fi

# 创建主题
echo -e "${GREEN}=== 创建主题 ===${NC}"
for topic in "${!TOPICS[@]}"; do
    IFS=' ' read -r read_queue write_queue <<< "${TOPICS[$topic]}"

    echo -e "${YELLOW}创建主题: $topic (读队列: $read_queue, 写队列: $write_queue)${NC}"

    if docker exec -it $BROKER_CONTAINER sh mqadmin updateTopic \
        -n $NAMESERVER \
        -t "$topic" \
        -c $CLUSTER \
        -r $read_queue \
        -w $write_queue > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 主题 $topic 创建成功${NC}"
    else
        echo -e "${RED}✗ 主题 $topic 创建失败${NC}"
    fi
done

echo ""

# 创建消费者组
echo -e "${GREEN}=== 创建消费者组 ===${NC}"
for group in "${CONSUMER_GROUPS[@]}"; do
    echo -e "${YELLOW}创建消费者组: $group${NC}"

    if docker exec -it $BROKER_CONTAINER sh mqadmin updateSubGroup \
        -n $NAMESERVER \
        -c $CLUSTER \
        -g "$group" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 消费者组 $group 创建成功${NC}"
    else
        echo -e "${RED}✗ 消费者组 $group 创建失败${NC}"
    fi
done

echo ""

# 显示主题列表
echo -e "${GREEN}=== 主题列表 ===${NC}"
docker exec -it $BROKER_CONTAINER sh mqadmin topicList -n $NAMESERVER 2>/dev/null | grep -E "solana-|pump-" || true

echo ""

# 显示消费者组列表
echo -e "${GREEN}=== 消费者组列表 ===${NC}"
docker exec -it $BROKER_CONTAINER sh mqadmin consumerProgress -n $NAMESERVER 2>/dev/null | grep "block-consumer" || true

echo ""
echo -e "${GREEN}=== 完成! ===${NC}"
echo -e "访问管理控制台: ${YELLOW}http://localhost:8090${NC}"
echo -e "NameServer 地址: ${YELLOW}127.0.0.1:9876${NC}"
