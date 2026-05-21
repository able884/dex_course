package logic

import (
	"context"

	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"

	"github.com/zeromicro/go-zero/core/logx"
)

// UploadTokenMetadataLogic 处理代币元数据上传业务逻辑（预留接口）
type UploadTokenMetadataLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewUploadTokenMetadataLogic 创建代币元数据上传逻辑处理器
func NewUploadTokenMetadataLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadTokenMetadataLogic {
	return &UploadTokenMetadataLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UploadTokenMetadata 上传代币元数据（预留接口）
// 当前实现直接委托给 UploadMetadataLogic 处理
// 该方法可以在未来扩展独立的代币元数据处理逻辑
func (l *UploadTokenMetadataLogic) UploadTokenMetadata(in *trade.UploadMetadataRequest) (*trade.UploadMetadataResponse, error) {
	// 委托给 UploadMetadataLogic 处理
	uml := NewUploadMetadataLogic(l.ctx, l.svcCtx)
	return uml.UploadTokenMetadata(in)
}
