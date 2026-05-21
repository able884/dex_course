package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"richcode.cc/dex/message/config"
	"richcode.cc/dex/message/internal/server"

	"github.com/zeromicro/go-zero/core/logx"
)

func main() {
	var configFile = flag.String("f", "etc/message.yaml", "the config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	tokenServer := server.NewTokenWebSocketServer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 开始启动订阅
	go tokenServer.StartSubscription(ctx)

	// go 标准库的http路由多路复用器，负责把请求分发到对应的处理函数
	mux := http.NewServeMux()
	// websocket连接接口，client通过这个接口连接服务器并订阅代币信息
	mux.HandleFunc("/ws/tokens", tokenServer.HandleTokenWebSocket)
	// 健康检查接口，返回json
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy", "service": "token-websocket"}`))
	})
	// 提供测试页面
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/test.html")
	})

	addr := tokenServer.GetServerAddress()
	// 初始化http服务
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// 异步起动http服务
	go func() {
		logx.Infof("Token WebSocket server starting on %s", addr)
		logx.Infof("WebSocket endpoint: ws://%s/ws/tokens?chain_id=100000", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 非正常关闭服务时，打印日志
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	// SIGINT-用户按下CTRL+C,SIGTERM-系统发送的终止信号（如kill命令），signal.Notify函数会将这些信号发送到quit通道
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// 阻塞，直到接收到quit信号，然后关闭http服务
	<-quit

	logx.Info("Shutting down token WebSocket server...")
	cancel()

	// 创建一个带5秒超时的上下文
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 函数退出时自动取消上下文
	defer shutdownCancel()

	// 关闭http服务
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logx.Info("Token WebSocket server exited gracefully")
}
