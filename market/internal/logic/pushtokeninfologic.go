package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// Redis 频道名称：新代币创建通知
	redisChannelPumpTokenNew = "pump_token_new"
)

// PushTokenInfoLogic 推送代币信息到WebSocket的业务逻辑处理器
// 该处理器从消费者接收代币元数据，通过数据库增强信息后推送到Redis

type PushTokenInfoLogic struct {
	ctx    context.Context              // 上下文对象
	svcCtx *svc.ServiceContext          // 服务上下文
	logx.Logger                          // 日志记录器
}

// NewPushTokenInfoLogic 创建推送代币信息的逻辑处理器
// 参数:
//   ctx - 上下文对象
//   svcCtx - 服务上下文
// 返回: 初始化后的PushTokenInfoLogic指针
func NewPushTokenInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PushTokenInfoLogic {
	return &PushTokenInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// PushTokenInfo 推送代币信息到 WebSocket
// 该方法从消费者接收代币元数据，并通过数据库增强信息后推送到 Redis
func (l *PushTokenInfoLogic) PushTokenInfo(in *market.PushTokenInfoRequest) (*market.PushTokenInfoResponse, error) {
	// 构建代币数据
	tokenData := l.buildTokenData(in)

	// 发布到 Redis
	if err := l.publishToRedis(tokenData); err != nil {
		l.Errorf("推送代币信息到 Redis 失败: %v", err)
		return l.buildErrorResponse(in), nil
	}

	return l.buildSuccessResponse(in), nil
}

// buildTokenData 构建完整的代币数据
func (l *PushTokenInfoLogic) buildTokenData(in *market.PushTokenInfoRequest) map[string]interface{} {
	// 从请求中获取基础信息
	tokenName := in.TokenName
	tokenSymbol := in.TokenSymbol
	tokenIcon := in.TokenIcon
	launchTime := l.getLaunchTime(in.LaunchTime)

	// 从数据库获取增强信息
	twitterUsername, telegram, holdCount := l.getEnhancedTokenInfo(in, &tokenName, &tokenIcon)

	return map[string]interface{}{
		"chain_id":         in.ChainId,
		"token_address":    in.TokenAddress,
		"pair_address":     in.PairAddress,
		"token_price":      in.TokenPrice,
		"mkt_cap":          in.MktCap,
		"token_name":       tokenName,
		"token_symbol":     tokenSymbol,
		"token_icon":       tokenIcon,
		"launch_time":      launchTime,
		"hold_count":       holdCount,
		"change_24":        in.Change_24,
		"txs_24h":          in.Txs_24H,
		"pump_status":      in.PumpStatus,
		"twitter_username": twitterUsername,
		"telegram":         telegram,
	}
}

// getLaunchTime 获取发布时间，如果未提供则使用当前时间
func (l *PushTokenInfoLogic) getLaunchTime(launchTime int64) int64 {
	if launchTime == 0 {
		return time.Now().Unix()
	}
	return launchTime
}

// getEnhancedTokenInfo 从数据库获取增强的代币信息
// 返回值: twitterUsername, telegram, holdCount
func (l *PushTokenInfoLogic) getEnhancedTokenInfo(in *market.PushTokenInfoRequest, tokenName, tokenIcon *string) (string, string, int64) {
	var twitterUsername, telegram string
	var holdCount int64

	// 查询数据库中的代币信息
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	tokenInfo, err := tokenModel.FindOneByChainIdAddress(l.ctx, in.ChainId, in.TokenAddress)
	if err != nil || tokenInfo == nil {
		l.Infof("未在数据库中找到代币信息，地址: %s, 错误: %v", in.TokenAddress, err)
		return "", "", 0
	}

	// 使用数据库中的值补充缺失的字段
	if *tokenName == "" {
		*tokenName = tokenInfo.Name
	}
	if *tokenIcon == "" {
		*tokenIcon = tokenInfo.Icon
	}

	// 获取社交媒体信息
	twitterUsername = tokenInfo.TwitterUsername
	telegram = tokenInfo.Telegram

	// 计算持有者数量
	holdCount = l.getHolderCount(in.ChainId, in.TokenAddress, tokenInfo.CreatedAt)

	return twitterUsername, telegram, holdCount
}

// getHolderCount 获取代币持有者数量
func (l *PushTokenInfoLogic) getHolderCount(chainId int64, tokenAddress string, createdAt time.Time) int64 {
	solTokenAccountModel := solmodel.NewSolTokenAccountModel(l.svcCtx.DB)
	holders, err := solTokenAccountModel.CountByTokenAddressWithTime(l.ctx, chainId, tokenAddress, createdAt)
	if err != nil {
		l.Errorf("统计代币持有者数量失败: %v", err)
		return 0
	}
	return holders
}

// publishToRedis 将代币数据发布到 Redis
func (l *PushTokenInfoLogic) publishToRedis(tokenData map[string]interface{}) error {
	// 转换为 JSON
	jsonData, err := json.Marshal(tokenData)
	if err != nil {
		l.Errorf("序列化代币数据失败: %v", err)
		return err
	}

	// 发布到 Redis 频道
	_, err = l.svcCtx.RDS.Publish(redisChannelPumpTokenNew, string(jsonData))
	if err != nil {
		return err
	}

	return nil
}

// buildSuccessResponse 构建成功响应
func (l *PushTokenInfoLogic) buildSuccessResponse(in *market.PushTokenInfoRequest) *market.PushTokenInfoResponse {
	return &market.PushTokenInfoResponse{
		ChainId:      in.ChainId,
		TokenAddress: in.TokenAddress,
		Txs_24H:      uint32(in.Txs_24H),
		Vol_24H:      "0",
		Change_24:    fmt.Sprintf("%.2f", in.Change_24),
		TokenPrice:   fmt.Sprintf("%.18f", in.TokenPrice),
		MktCap:       fmt.Sprintf("%.2f", in.MktCap),
	}
}

// buildErrorResponse 构建错误响应
func (l *PushTokenInfoLogic) buildErrorResponse(in *market.PushTokenInfoRequest) *market.PushTokenInfoResponse {
	return &market.PushTokenInfoResponse{
		ChainId:      in.ChainId,
		TokenAddress: in.TokenAddress,
		Txs_24H:      0,
		Vol_24H:      "0",
		Change_24:    "0",
		TokenPrice:   "0",
		MktCap:       "0",
	}
}
