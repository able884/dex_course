package logic

import (
	"context"
	"fmt"

	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetTokenInfoLogic 获取代币详细信息的业务逻辑处理器
type GetTokenInfoLogic struct {
	ctx    context.Context              // 上下文对象
	svcCtx *svc.ServiceContext          // 服务上下文
	logx.Logger                          // 日志记录器
}

// NewGetTokenInfoLogic 创建获取代币详细信息的逻辑处理器
// 参数:
//   ctx - 上下文对象
//   svcCtx - 服务上下文
// 返回: 初始化后的GetTokenInfoLogic指针
func NewGetTokenInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTokenInfoLogic {
	return &GetTokenInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetTokenInfo 获取代币详细信息的主方法
// 返回代币的基本信息、安全分析、社交媒体链接等
// 参数:
//   in - 请求参数，包含链ID和代币地址
// 返回:
//   *market.GetTokenInfoResponse - 代币详细信息响应
//   error - 错误信息
func (l *GetTokenInfoLogic) GetTokenInfo(in *market.GetTokenInfoRequest) (*market.GetTokenInfoResponse, error) {
	// 验证请求参数：请求对象和代币地址不能为空
	if in == nil || in.TokenAddress == "" {
		return nil, fmt.Errorf("token address is required")
	}
	// 验证请求参数：链ID不能为0
	if in.ChainId == 0 {
		return nil, fmt.Errorf("chain id is required")
	}

	// 创建代币数据模型
	tokenModel := solmodel.NewTokenModel(l.svcCtx.DB)
	// 根据链ID和代币地址查询代币信息
	token, err := tokenModel.FindOneByChainIdAddress(l.ctx, in.ChainId, in.TokenAddress)
	if err != nil {
		// 查询失败，记录错误日志并返回错误
		l.Errorf("GetTokenInfo FindOneByChainIdAddress failed, chainId:%d, address:%s, err:%v", in.ChainId, in.TokenAddress, err)
		return nil, err
	}
	// 检查代币是否存在
	if token == nil {
		return nil, fmt.Errorf("token not found, chainId:%d, address:%s", in.ChainId, in.TokenAddress)
	}

	// 构建并返回代币详细信息响应
	return &market.GetTokenInfoResponse{
		ChainId:           token.ChainId,              // 链ID
		Address:           token.Address,              // 代币地址
		Name:              token.Name,                 // 代币名称
		Symbol:            token.Symbol,               // 代币符号
		Decimals:          token.Decimals,             // 小数位数
		TotalSupply:       token.TotalSupply,          // 总供应量
		Icon:              token.Icon,                 // 代币图标
		HoldCount:         token.HoldCount,            // 持有者数量
		IsCaDropOwner:     token.IsCaDropOwner,        // 是否已放弃合约所有权
		IsCaVerify:        token.IsCaVerify,           // 合约是否已验证
		IsHoneyScam:       token.IsHoneyScam,          // 是否为蜜罐（无法卖出）
		IsLiquidLock:      token.IsLiquidLock,         // 流动性是否已锁定
		IsCanPauseTrade:   token.IsCanPauseTrade,      // 是否可以暂停交易
		IsCanChangeTax:    token.IsCanChangeTax,       // 是否可以修改税率
		IsHaveBlackList:   token.IsHaveBlackList,      // 是否有黑名单机制
		IsCanAllSell:      token.IsCanAllSell,         // 是否可以全部卖出
		IsHaveProxy:       token.IsHaveProxy,          // 是否有代理合约
		IsCanExternalCall: token.IsCanExternalCall,    // 合约是否可以外部调用
		IsCanAddToken:     token.IsCanAddToken,        // 是否有增发能力
		IsCanChangeToken:  token.IsCanChangeToken,     // 所有者是否可以修改用户余额
		SellTax:           token.SellTax,              // 卖出税率
		BuyTax:            token.BuyTax,               // 买入税率
		TwitterUsername:   token.TwitterUsername,      // Twitter用户名
		Website:           token.Website,              // 官方网站
		Telegram:          token.Telegram,             // Telegram链接
		IsCheckCa:         token.IsCheckCa,            // 合约分析是否已完成
		CheckCaAt:         token.CheckCaAt,            // 合约分析时间
		Program:           token.Program,              // 代币所属程序
	}, nil
}
