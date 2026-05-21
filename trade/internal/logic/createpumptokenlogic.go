package logic

import (
	"context"
	"fmt"
	"time"

	ag_solanago "github.com/gagliardetto/solana-go"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/pumpfun"
	"richcode.cc/dex/pkg/rediskeys"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// CreatePumpTokenLogic 处理 PumpFun 代币创建业务逻辑
type CreatePumpTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewCreatePumpTokenLogic 创建 PumpFun 代币创建逻辑处理器
func NewCreatePumpTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePumpTokenLogic {
	return &CreatePumpTokenLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

// CreatePumpToken 构建 PumpFun 代币创建的未签名交易并返回给客户端
// 该方法负责：
// 1. 参数校验（链ID、必填字段）
// 2. 如果未提供 URI，则自动生成并上传代币元数据到 IPFS
// 3. 构建未签名的代币创建交易
// 4. 持久化未签名交易记录（用于后续回调确认）
// 5. 返回 base64 编码的未签名交易给客户端
func (l *CreatePumpTokenLogic) CreatePumpToken(in *trade.CreatePumpTokenRequest) (*trade.CreatePumpTokenResponse, error) {
	// 1. 参数校验：请求不能为空
	if in == nil {
		return nil, fmt.Errorf("请求为空")
	}

	// 2. 校验链ID必须是 Solana
	if in.ChainId != int32(constants.SolChainIdInt) && in.ChainId != 100000 {
		return nil, fmt.Errorf("不支持的链ID: %d", in.ChainId)
	}

	// 3. 校验 SolTxMananger 是否已初始化
	if l.svcCtx.SolTxMananger == nil {
		return nil, fmt.Errorf("SolTxMananger 未初始化")
	}

	// 4. 校验必填字段
	if in.Name == "" || in.Symbol == "" || in.Mint == "" || in.UserWalletAddress == "" {
		return nil, fmt.Errorf("缺少必填字段")
	}

	// 5. 如果未提供 URI，则自动生成元数据并上传到 IPFS
	if in.Uri == "" {
		uml := NewUploadMetadataLogic(l.ctx, l.svcCtx)
		metaResp, err := uml.UploadTokenMetadata(&trade.UploadMetadataRequest{
			Name:               in.Name,
			Symbol:             in.Symbol,
			Description:        in.Description,
			ImageUri:           in.ImageUri,
			ImageContentBase64: in.ImageContentBase64,
			Website:            in.Website,
			Twitter:            in.Twitter,
			Telegram:           in.Telegram,
		})
		if err != nil {
			return nil, fmt.Errorf("元数据上传失败: %w", err)
		}
		// 克隆请求并填充 URI
		cloned := *in
		cloned.Uri = metaResp.Uri
		cloned.ImageUri = metaResp.ImageUri
		in = &cloned
	}

	// 6. 构建未签名的代币创建交易
	txBase64, err := l.svcCtx.SolTxMananger.BuildUnsignedPumpCreateTransaction(l.ctx, in)
	if err != nil {
		l.Errorf("BuildUnsignedPumpCreateTransaction 失败: %v", err)
		return nil, err
	}

	// 7. 派生交易对地址（用于后续追踪）
	pairAddr := ""
	if mintPubKey, err := ag_solanago.PublicKeyFromBase58(in.Mint); err != nil {
		l.Errorf("无效的 mint 地址: %v", err)
	} else if bondingCurve, _, err := pumpfun.GetBondingCurvePDA(mintPubKey); err != nil {
		l.Errorf("派生交易对 PDA 失败: %v", err)
	} else {
		pairAddr = bondingCurve.String()
	}

	// 8. 持久化未签名交易记录（用于回调确认）
	// 该记录用于在客户端上报交易结果后，完成代币和交易对数据的写入
	if l.svcCtx.DB != nil {
		model := solmodel.NewPumpCreateTokenUnsignedModel(l.svcCtx.DB)
		rec := &solmodel.PumpCreateTokenUnsigned{
			ChainId:           int64(in.ChainId),
			Mint:              in.Mint,
			UserWalletAddress: in.UserWalletAddress,
			Name:              in.Name,
			Symbol:            in.Symbol,
			Uri:               in.Uri,
			Website:           in.Website,
			Twitter:           in.Twitter,
			Telegram:          in.Telegram,
			Description:       in.Description,
			ImageUri:          in.ImageUri,
			UnsignedTx:        txBase64,
			DexName:           constants.PumpFun,
			PairAddr:          pairAddr,
			Status:            0, // 0-处理中, 1-成功, 2-失败
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
		if err := model.Insert(l.ctx, rec); err != nil {
			// 持久化失败不阻止返回交易给客户端，仅记录日志
			l.Errorf("插入未签名 Pump 交易记录失败: %v", err)
		} else {
			// 在 Redis 中标记交易对创建状态
			l.markPumpPairCreation(int64(in.ChainId), pairAddr)
		}
	}

	// 9. 返回未签名交易给客户端
	return &trade.CreatePumpTokenResponse{Tx: txBase64}, nil
}

// markPumpPairCreation 在 Redis 中标记 Pump 交易对创建状态
// 参数：
//   - chainId: 链ID
//   - pairAddr: 交易对地址
func (l *CreatePumpTokenLogic) markPumpPairCreation(chainId int64, pairAddr string) {
	if pairAddr == "" || l.svcCtx.Redis == nil {
		return
	}

	// 设置过期时间
	ttlSeconds := int(rediskeys.PumpPairCreatedTTL.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 600 // 默认10分钟
	}

	// 设置 Redis key
	key := rediskeys.PumpPairCreatedKey(chainId, pairAddr)
	if err := l.svcCtx.Redis.Setex(key, "1", ttlSeconds); err != nil {
		l.Errorf("设置 Pump 交易对 Redis 标记失败: key=%s, err=%v", key, err)
	}
}