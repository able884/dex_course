@echo off
REM RocketMQ Topic Creation Script (Windows)
REM Automatically creates required topics and consumer groups

setlocal enabledelayedexpansion

echo === RocketMQ Topic and Consumer Group Creation Script ===
echo.

REM Configuration
set NAMESERVER=rocketmq-nameserver:9876
set CLUSTER=DefaultCluster
set BROKER_CONTAINER=rocketmq-broker

echo Checking RocketMQ container status...
docker ps | findstr "%BROKER_CONTAINER%" >nul 2>&1
if errorlevel 1 (
    echo Error: RocketMQ Broker container is not running
    echo Please start RocketMQ first: docker-compose -f docker-compose-rocketmq.yml up -d
    exit /b 1
)

echo Waiting for Broker to start...
timeout /t 5 /nobreak >nul

echo Checking NameServer connection...
docker exec -it %BROKER_CONTAINER% sh mqadmin clusterList -n %NAMESERVER% >nul 2>&1
if errorlevel 1 (
    echo Failed to connect to NameServer
    exit /b 1
)
echo NameServer connection successful
echo.

echo === Creating Topics ===

REM Create topic: solana-blocks
echo Creating topic: solana-blocks
docker exec -it %BROKER_CONTAINER% sh mqadmin updateTopic -n %NAMESERVER% -t solana-blocks -c %CLUSTER% -r 8 -w 8 >nul 2>&1
if errorlevel 1 (
    echo [Failed] solana-blocks
) else (
    echo [Success] solana-blocks
)

REM Create topic: solana-blocks-retry
echo Creating topic: solana-blocks-retry
docker exec -it %BROKER_CONTAINER% sh mqadmin updateTopic -n %NAMESERVER% -t solana-blocks-retry -c %CLUSTER% -r 8 -w 8 >nul 2>&1
if errorlevel 1 (
    echo [Failed] solana-blocks-retry
) else (
    echo [Success] solana-blocks-retry
)

REM Create topic: pump-migration
echo Creating topic: pump-migration
docker exec -it %BROKER_CONTAINER% sh mqadmin updateTopic -n %NAMESERVER% -t pump-migration -c %CLUSTER% -r 4 -w 4 >nul 2>&1
if errorlevel 1 (
    echo [Failed] pump-migration
) else (
    echo [Success] pump-migration
)

REM Create DLQ topic (Dead Letter Queue)
echo Creating topic: %%DLQ%%block-consumer-group
docker exec -it %BROKER_CONTAINER% sh mqadmin updateTopic -n %NAMESERVER% -t "%%DLQ%%block-consumer-group" -c %CLUSTER% -r 1 -w 1 >nul 2>&1
if errorlevel 1 (
    echo [Failed] %%DLQ%%block-consumer-group
) else (
    echo [Success] %%DLQ%%block-consumer-group
)

echo.
echo === Creating Consumer Groups ===

REM Create consumer group: block-consumer-group
echo Creating consumer group: block-consumer-group
docker exec -it %BROKER_CONTAINER% sh mqadmin updateSubGroup -n %NAMESERVER% -c %CLUSTER% -g block-consumer-group >nul 2>&1
if errorlevel 1 (
    echo [Failed] block-consumer-group
) else (
    echo [Success] block-consumer-group
)

REM Create consumer group: block-consumer-group-dlq-handler
echo Creating consumer group: block-consumer-group-dlq-handler
docker exec -it %BROKER_CONTAINER% sh mqadmin updateSubGroup -n %NAMESERVER% -c %CLUSTER% -g block-consumer-group-dlq-handler >nul 2>&1
if errorlevel 1 (
    echo [Failed] block-consumer-group-dlq-handler
) else (
    echo [Success] block-consumer-group-dlq-handler
)

echo.
echo === Topic List ===
docker exec -it %BROKER_CONTAINER% sh mqadmin topicList -n %NAMESERVER% 2>nul | findstr /I "solana pump"

echo.
echo === Done! ===
echo Access management console: http://localhost:8090
echo NameServer address: 127.0.0.1:9876

pause
