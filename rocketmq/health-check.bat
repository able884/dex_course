@echo off
REM RocketMQ Health Check Script (Windows)

setlocal enabledelayedexpansion

set NAMESERVER=rocketmq-nameserver:9876
set BROKER_CONTAINER=rocketmq-broker

echo === RocketMQ Health Check ===
echo.

REM 1. Check container status
echo [1/6] Checking Docker container status...
docker ps --filter "name=rocketmq" --format "table {{.Names}}\t{{.Status}}" | findstr "Up" >nul 2>&1
if errorlevel 1 (
    echo [Failed] Containers are not running or unhealthy
    exit /b 1
) else (
    echo [Success] Containers are running normally
    docker ps --filter "name=rocketmq" --format "table {{.Names}}\t{{.Status}}"
)
echo.

REM 2. Check NameServer
echo [2/6] Checking NameServer connection...
docker exec -it %BROKER_CONTAINER% sh mqadmin clusterList -n %NAMESERVER% >nul 2>&1
if errorlevel 1 (
    echo [Failed] NameServer connection failed
    exit /b 1
) else (
    echo [Success] NameServer connection successful
)
echo.

REM 3. Check topics
echo [3/6] Checking project topics...
docker exec -it %BROKER_CONTAINER% sh mqadmin topicList -n %NAMESERVER% 2>nul > topic_list.tmp

findstr /I "solana-blocks" topic_list.tmp >nul 2>&1
if errorlevel 1 (
    echo [Missing] solana-blocks
) else (
    echo [Exists] solana-blocks
)

findstr /I "solana-blocks-retry" topic_list.tmp >nul 2>&1
if errorlevel 1 (
    echo [Missing] solana-blocks-retry
) else (
    echo [Exists] solana-blocks-retry
)

findstr /I "pump-migration" topic_list.tmp >nul 2>&1
if errorlevel 1 (
    echo [Missing] pump-migration
) else (
    echo [Exists] pump-migration
)

del topic_list.tmp
echo.

REM 4. Check consumer groups
echo [4/6] Checking consumer groups...
docker exec -it %BROKER_CONTAINER% sh mqadmin consumerProgress -n %NAMESERVER% 2>nul | findstr /I "block-consumer-group" >nul 2>&1
if errorlevel 1 (
    echo [Warning] block-consumer-group might not exist
) else (
    echo [Exists] block-consumer-group
)

docker exec -it %BROKER_CONTAINER% sh mqadmin consumerProgress -n %NAMESERVER% 2>nul | findstr /I "block-consumer-group-dlq-handler" >nul 2>&1
if errorlevel 1 (
    echo [Warning] block-consumer-group-dlq-handler might not exist
) else (
    echo [Exists] block-consumer-group-dlq-handler
)
echo.

REM 5. Check Broker configuration
echo [5/6] Checking Broker configuration...
docker exec -it %BROKER_CONTAINER% sh mqadmin getBrokerConfig -n %NAMESERVER% -b rocketmq-broker:10911 2>nul > broker_config.tmp

findstr /I "maxMessageSize.*16777216" broker_config.tmp >nul 2>&1
if errorlevel 1 (
    echo [Warning] Max message size configuration might be incorrect
) else (
    echo [Success] Max message size: 16MB
)

findstr /I "autoCreateTopicEnable.*true" broker_config.tmp >nul 2>&1
if errorlevel 1 (
    echo [Warning] Auto create topic is not enabled
) else (
    echo [Success] Auto create topic: Enabled
)

del broker_config.tmp
echo.

REM 6. Check management console
echo [6/6] Checking management console...
curl -s http://localhost:8090 >nul 2>&1
if errorlevel 1 (
    echo [Warning] Console is not accessible
) else (
    echo [Success] Console is accessible: http://localhost:8090
)
echo.

REM Summary
echo === Health Check Completed ===
echo.
echo System Information:
echo   - NameServer: 127.0.0.1:9876
echo   - Management Console: http://localhost:8090
echo   - Broker: rocketmq-broker:10911
echo.
echo For help, please see: rocketmq\README.md

pause
