package logic

import (
	"context"
	"fmt"
	"time"

	"richcode.cc/dex/model/trademodel"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/internal/types"
	"richcode.cc/dex/trade/trade"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetCpmmFeeTiersLogic 处理 CPMM 手续费等级查询业务逻辑
type GetCpmmFeeTiersLogic struct {
	ctx     context.Context
	svcCtx  *svc.ServiceContext
	metrics types.LiquidityMetrics
	logx.Logger
}

// NewGetCpmmFeeTiersLogic 创建 CPMM 手续费等级查询逻辑处理器
func NewGetCpmmFeeTiersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCpmmFeeTiersLogic {
	metrics := svcCtx.LiquidityMetric
	if metrics == nil {
		metrics = types.NoopLiquidityMetrics{}
	}
	return &GetCpmmFeeTiersLogic{
		ctx:     ctx,
		svcCtx:  svcCtx,
		metrics: metrics,
		Logger:  logx.WithContext(ctx),
	}
}

// GetCpmmFeeTiers 查询 CPMM 手续费等级列表
// 该方法负责：
// 1. 构建查询参数
// 2. 调用底层查询方法
// 3. 转换返回结果格式
func (l *GetCpmmFeeTiersLogic) GetCpmmFeeTiers(in *trade.GetCpmmFeeTiersRequest) (*trade.GetCpmmFeeTiersResponse, error) {
	// 1. 构建查询参数
	resp, err := l.calcCpmmFeeTiers(&types.GetCpmmFeeTiersInput{
		PoolType: in.GetPoolType(),
	})
	if err != nil {
		return nil, err
	}

	// 2. 转换结果格式
	items := make([]*trade.CpmmFeeTierItem, 0, len(resp.Tiers))
	for _, tier := range resp.Tiers {
		items = append(items, &trade.CpmmFeeTierItem{
			PoolType:       tier.PoolType,
			ValueBps:       tier.ValueBps,
			Label:          tier.Label,
			Description:    tier.Description,
			TickSpacing:    tier.TickSpacing,
			PriorityOrder:  tier.PriorityOrder,
			Version:        tier.Version,
			UpdatedAtIso:   tier.UpdatedAt.UTC().Format(time.RFC3339),
			UpdatedAtUnix:  tier.UpdatedAt.Unix(),
			ProgramAddress: tier.ProgramAddress,
			ConfigIndex:    tier.ConfigIndex,
			Address:        tier.Address,
		})
	}

	// 3. 返回结果
	return &trade.GetCpmmFeeTiersResponse{
		Tiers:   items,
		Version: resp.Version,
	}, nil
}

// calcCpmmFeeTiers 查询 CPMM 手续费等级（内部方法）
// 该方法负责：
// 1. 参数标准化
// 2. 从数据库查询手续费等级
// 3. 计算最新版本号
func (l *GetCpmmFeeTiersLogic) calcCpmmFeeTiers(in *types.GetCpmmFeeTiersInput) (*types.GetCpmmFeeTiersOutput, error) {
	start := time.Now()
	status := "success"

	// 记录查询耗时指标
	defer func() {
		l.metrics.RecordFeeFetch(status, time.Since(start))
	}()

	// 参数标准化
	if in == nil {
		in = &types.GetCpmmFeeTiersInput{}
	}
	if in.PoolType == "" {
		in.PoolType = "CPMM"
	}

	// 从数据库查询手续费等级
	var records []trademodel.CpmmFeeTier
	if err := l.svcCtx.DB.WithContext(l.ctx).
		Model(&trademodel.CpmmFeeTier{}).
		Where("pool_type = ?", in.PoolType).
		Order("priority_order ASC").
		Find(&records).Error; err != nil {
		status = "error"
		return nil, err
	}

	// 检查是否有数据
	if len(records) == 0 {
		status = "error"
		return nil, fmt.Errorf("没有可用的手续费等级，pool_type: %s", in.PoolType)
	}

	// 计算最新版本号并转换结果
	var latestVersion string
	var latest time.Time
	tiers := make([]*types.CpmmFeeTier, 0, len(records))
	for _, rec := range records {
		if rec.UpdatedAt.After(latest) {
			latest = rec.UpdatedAt
			latestVersion = rec.Version
		}
		tiers = append(tiers, &types.CpmmFeeTier{
			PoolType:       rec.PoolType,
			ProgramAddress: rec.ProgramAddress,
			ValueBps:       int32(rec.ValueBps),
			Label:          rec.Label,
			Description:    rec.Description,
			TickSpacing:    int32(rec.TickSpacing),
			ConfigIndex:    int32(rec.ConfigIndex),
			Address:        rec.Address,
			PriorityOrder:  int32(rec.PriorityOrder),
			Version:        rec.Version,
			UpdatedAt:      rec.UpdatedAt,
		})
	}

	return &types.GetCpmmFeeTiersOutput{
		Tiers:   tiers,
		Version: latestVersion,
	}, nil
}