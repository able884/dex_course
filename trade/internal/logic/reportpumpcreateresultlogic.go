package logic

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// ReportPumpCreateResultLogic 处理 PumpFun 代币创建结果上报业务逻辑
type ReportPumpCreateResultLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewReportPumpCreateResultLogic 创建 PumpFun 代币创建结果上报逻辑处理器
func NewReportPumpCreateResultLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportPumpCreateResultLogic {
	return &ReportPumpCreateResultLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// ReportPumpCreateResult 上报 PumpFun 代币创建结果
// 该方法负责：
// 1. 参数校验（链ID、代币地址、用户钱包等必填字段）
// 2. 获取基础代币价格（用于计算 FDV 和流动性）
// 3. 调用 CompletePumpCreateBySignature 完成代币创建后续处理
// 4. 返回交易对地址和代币地址给客户端
func (l *ReportPumpCreateResultLogic) ReportPumpCreateResult(in *trade.ReportPumpCreateResultRequest) (*trade.ReportPumpCreateResultResponse, error) {
	// 1. 校验请求不能为空
	if in == nil {
		return nil, fmt.Errorf("nil request")
	}

	// 2. 校验必填字段
	if in.ChainId == 0 || in.Mint == "" || in.UserWalletAddress == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	// 3. 成功情况下必须提供交易签名
	if in.Success && in.TxSignature == "" {
		return nil, fmt.Errorf("missing tx_signature when success=true")
	}

	// 4. 校验服务是否初始化
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.DB == nil {
		return nil, fmt.Errorf("service not initialized")
	}

	// 5. 获取基础代币价格（用于计算 FDV 和流动性）
	basePrice := 0.0
	if l.svcCtx.MarketClient != nil {
		resp, err := l.svcCtx.MarketClient.GetNativeTokenPrice(l.ctx, &market.GetNativeTokenPriceRequest{
			ChainId: int64(in.ChainId),
		})
		if err != nil {
			l.Errorf("GetNativeTokenPrice err: %v", err)
		} else {
			basePrice = resp.BaseTokenPriceUsd
		}
	}

	// 6. 获取 IPFS 网关和 RPC 节点配置
	ipfsGateway := l.svcCtx.IpfsGateway
	nodeUrls := l.svcCtx.Config.SolConfig.NodeUrl
	if len(nodeUrls) == 0 {
		return nil, fmt.Errorf("Sol node url not configured")
	}

	// 7. 完成代币创建后续处理（更新未签名交易记录、写入交易对和代币数据）
	err := CompletePumpCreateBySignature(
		l.ctx,
		l.svcCtx.DB,
		nodeUrls[0],
		l.svcCtx.SolTxMananger.Client,
		int64(in.ChainId),
		in.Mint,
		in.UserWalletAddress,
		in.TxSignature,
		in.Success,
		in.ErrorMessage,
		basePrice,
		ipfsGateway,
	)
	if err != nil {
		l.Errorf("CompletePumpCreateBySignature err: %v", err)
		return &trade.ReportPumpCreateResultResponse{Status: "fail"}, nil
	}

	// 8. 从数据库读取交易对地址（用于返回给客户端）
	unsignedModel := solmodel.NewPumpCreateTokenUnsignedModel(l.svcCtx.DB)
	rec, _ := unsignedModel.FindOneByChainIdMint(l.ctx, int64(in.ChainId), in.Mint)
	pairAddr := ""
	if rec != nil {
		pairAddr = rec.PairAddr
	}

	// 9. 返回成功响应
	return &trade.ReportPumpCreateResultResponse{Status: "ok", PairAddr: pairAddr, TokenAddress: in.Mint}, nil
}
