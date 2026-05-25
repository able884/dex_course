@echo off
REM Quick restart script for RocketMQ (Windows)

echo === Restarting RocketMQ ===
echo.

echo [1/3] Stopping RocketMQ containers...
docker-compose -f ..\docker-compose-rocketmq.yml stop
echo.

echo [2/3] Starting RocketMQ containers...
docker-compose -f ..\docker-compose-rocketmq.yml up -d
echo.

echo [3/3] Waiting for services to start...
timeout /t 10 /nobreak >nul

echo.
echo === Checking service status ===
docker-compose -f ..\docker-compose-rocketmq.yml ps

echo.
echo === RocketMQ Restarted Successfully ===
echo NameServer: 127.0.0.1:9876
echo Console: http://localhost:8090
echo.
echo Wait 30 seconds before starting your application.

pause
