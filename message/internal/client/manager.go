package client

import (
	"encoding/json"
	"sync"
	"time"

	"richcode.cc/dex/message/internal/types"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

// Manager 管理WebSocket客户端连接
type Manager struct {
	clients   map[*websocket.Conn]*types.TokenClient
	clientsMu sync.RWMutex
}

// NewManager 创建一个新的客户端管理器
func NewManager() *Manager {
	return &Manager{
		clients: make(map[*websocket.Conn]*types.TokenClient),
	}
}

// AddClient 向管理器添加一个新客户端
func (m *Manager) AddClient(conn *websocket.Conn, chainId int64) *types.TokenClient {
	client := &types.TokenClient{
		Conn:       conn,
		ChainId:    chainId,
		Categories: []string{"new_creation", "completing", "completed"},
		Send:       make(chan []byte, 256),
	}

	m.clientsMu.Lock()
	m.clients[conn] = client
	m.clientsMu.Unlock()

	logx.Infof("[TOKEN WS] Client connected: chain=%d", chainId)
	return client
}

// RemoveClient 从管理器中移除一个客户端
func (m *Manager) RemoveClient(conn *websocket.Conn) {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	if client, exists := m.clients[conn]; exists {
		close(client.Send)
		delete(m.clients, conn)
		logx.Infof("[TOKEN WS] Client disconnected: chain=%d", client.ChainId)
	}
}

// GetClients 返回所有客户端（只读操作）
func (m *Manager) GetClients() map[*websocket.Conn]*types.TokenClient {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	// 返回副本以避免竞态条件
	clientsCopy := make(map[*websocket.Conn]*types.TokenClient, len(m.clients))
	for conn, client := range m.clients {
		clientsCopy[conn] = client
	}
	return clientsCopy
}

// SendWelcomeMessage 向新连接的客户端发送欢迎消息
func (m *Manager) SendWelcomeMessage(client *types.TokenClient) {
	welcomeMsg := types.TokenMessage{
		Type: "connection_established",
		Data: map[string]interface{}{
			"chain_id":   client.ChainId,
			"categories": client.Categories,
			"status":     "connected",
			"timestamp":  time.Now().Unix(),
		},
	}

	data, _ := json.Marshal(welcomeMsg)
	select {
	case client.Send <- data:
	default:
		logx.Errorf("[TOKEN WS] Failed to send welcome message: channel full")
	}
}

// WriteLoop 处理向客户端写入消息
func (m *Manager) WriteLoop(client *types.TokenClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		// 分支1：从send通道读取待发送的消息
		case message, ok := <-client.Send:
			// 1.为本次操作设置10秒超时
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// 2.如果通道关闭(客户端断开，服务器主动关闭)，则发送关闭帧并退出
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 3.发送文本消息给客户端
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logx.Errorf("[TOKEN WS] Write error: chain=%d, err=%v", client.ChainId, err)
				return
			}

		// 分支2：定时发送ping消息
		case <-ticker.C:
			// 为心跳设置10秒超时
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			// 发送ping消息，维持连接
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logx.Errorf("[TOKEN WS] Ping error: chain=%d, err=%v", client.ChainId, err)
				return
			}
		}
	}
}

// ReadLoop 处理从客户端读取消息
func (m *Manager) ReadLoop(client *types.TokenClient) {
	// 无论何种方法退出都执行（正常结束，报错，panic）
	defer func() {
		m.RemoveClient(client.Conn)
		client.Conn.Close()
	}()

	// 设置Pong心跳处理器，接受心跳维持链接
	client.Conn.SetPongHandler(func(string) error {
		return nil
	})

	for {
		// 阻塞读取消息
		messageType, message, err := client.Conn.ReadMessage()
		if err != nil {
			// CloseGoingAway-1001-客户端离开
			// CloseAbnormalClosure-1006-连接断开
			// CloseNormalClosure-1000-正常关闭
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				logx.Errorf("[TOKEN WS] Unexpected close: chain=%d, err=%v", client.ChainId, err)
			}
			break
		}

		if messageType == websocket.TextMessage {
			var subMsg types.SubscriptionMessage
			if err := json.Unmarshal(message, &subMsg); err == nil && subMsg.Type == "subscribe" {
				client.Categories = subMsg.Data.Categories
				logx.Infof("[TOKEN WS] Subscription updated: chain=%d, categories=%v", client.ChainId, client.Categories)
			}
		}
	}
}
