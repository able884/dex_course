package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"richcode.cc/dex/message/config"
	"richcode.cc/dex/message/internal/broadcast"
	"richcode.cc/dex/message/internal/client"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"
)

// TokenWebSocketServer 管理WebSocket连接和代币广播
type TokenWebSocketServer struct {
	clientManager *client.Manager
	broadcaster   *broadcast.Broadcaster
	upgrader      websocket.Upgrader
	config        config.Config
}

// NewTokenWebSocketServer 创建一个新的代币WebSocket服务器
func NewTokenWebSocketServer(cfg config.Config) *TokenWebSocketServer {
	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)

	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	clientManager := client.NewManager()

	broadcaster := broadcast.NewBroadcaster(
		redisClient,
		clientManager.GetClients,
		clientManager.RemoveClient,
	)

	return &TokenWebSocketServer{
		clientManager: clientManager,
		broadcaster:   broadcaster,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO: 生产环境需要限制
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		config: cfg,
	}
}

// HandleTokenWebSocket 处理WebSocket连接请求
func (s *TokenWebSocketServer) HandleTokenWebSocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Errorf("[TOKEN WS] Upgrade failed: %v", err)
		return
	}

	chainIdStr := r.URL.Query().Get("chain_id")
	if chainIdStr == "" {
		chainIdStr = "100000"
	}

	chainId, err := strconv.ParseInt(chainIdStr, 10, 64)
	if err != nil {
		logx.Errorf("[TOKEN WS] Invalid chain_id: %s", chainIdStr)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error": "Invalid chain_id"}`))
		conn.Close()
		return
	}

	tokenClient := s.clientManager.AddClient(conn, chainId)
	s.clientManager.SendWelcomeMessage(tokenClient)

	go s.clientManager.WriteLoop(tokenClient)
	go s.clientManager.ReadLoop(tokenClient)
}

// StartSubscription 启动Redis订阅服务
func (s *TokenWebSocketServer) StartSubscription(ctx context.Context) {
	s.broadcaster.StartSubscription(ctx)
}

// GetServerAddress 返回服务器地址
func (s *TokenWebSocketServer) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}
