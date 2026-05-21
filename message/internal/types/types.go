package types

import (
	"github.com/gorilla/websocket"
)

// TokenClient 表示一个WebSocket客户端连接
type TokenClient struct {
	Conn       *websocket.Conn
	ChainId    int64
	Categories []string // new_creation(新创建), completing(进行中), completed(已完成)
	Send       chan []byte
}

// TokenMessage 是所有WebSocket消息的包装器
type TokenMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// TokenData 包含代币信息
type TokenData struct {
	ChainId       int64   `json:"chain_id"`
	TokenAddress  string  `json:"token_address"`
	PairAddress   string  `json:"pair_address"`
	TokenName     string  `json:"token_name"`
	TokenSymbol   string  `json:"token_symbol"`
	TokenIcon     string  `json:"token_icon"`
	LaunchTime    int64   `json:"launch_time"`
	MktCap        float64 `json:"mkt_cap"`
	HoldCount     int64   `json:"hold_count"`
	Change24      float64 `json:"change_24"`
	Txs24h        int64   `json:"txs_24h"`
	PumpStatus    int     `json:"pump_status"` // 1=新创建, 2=进行中, 4=已完成
	OldPumpStatus int     `json:"old_pump_status,omitempty"`
}

// SubscriptionMessage 由客户端发送以更新其订阅偏好
type SubscriptionMessage struct {
	Type string `json:"type"`
	Data struct {
		ChainId    int64    `json:"chain_id"`
		Categories []string `json:"categories"`
	} `json:"data"`
}

// PumpStatusToCategory 将pump状态转换为类别字符串
func PumpStatusToCategory(pumpStatus int) string {
	switch pumpStatus {
	case 1:
		return "new_creation"
	case 2:
		return "completing"
	case 4:
		return "completed"
	default:
		return "new_creation"
	}
}

// IsSubscribedToCategory 检查客户端是否订阅了特定类别
func (c *TokenClient) IsSubscribedToCategory(category string) bool {
	for _, cat := range c.Categories {
		if cat == category {
			return true
		}
	}
	return false
}
