package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/internal/types"
	"richcode.cc/dex/trade/trade"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetCpmmTokensLogic 处理 CPMM 代币列表查询业务逻辑
type GetCpmmTokensLogic struct {
	ctx     context.Context
	svcCtx  *svc.ServiceContext
	metrics types.LiquidityMetrics
	logx.Logger
}

// NewGetCpmmTokensLogic 创建 CPMM 代币列表查询逻辑处理器
func NewGetCpmmTokensLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCpmmTokensLogic {
	metrics := svcCtx.LiquidityMetric
	if metrics == nil {
		metrics = types.NoopLiquidityMetrics{}
	}
	return &GetCpmmTokensLogic{
		ctx:     ctx,
		svcCtx:  svcCtx,
		metrics: metrics,
		Logger:  logx.WithContext(ctx),
	}
}

// GetCpmmTokens 查询 CPMM 允许交易的代币列表
// 该方法负责：
// 1. 参数标准化
// 2. 调用底层查询方法
// 3. 转换返回结果格式
func (l *GetCpmmTokensLogic) GetCpmmTokens(in *trade.GetCpmmTokensRequest) (*trade.GetCpmmTokensResponse, error) {
	// 1. 构建查询参数
	req := &types.GetCpmmTokensInput{
		ChainID:  int64(in.GetChainId()),
		Statuses: in.GetStatuses(),
		PageNo:   int64(in.GetPageNo()),
		PageSize: int64(in.GetPageSize()),
	}

	// 设置更新时间过滤条件
	if in.GetUpdatedAfterUnix() > 0 {
		ts := time.Unix(in.GetUpdatedAfterUnix(), 0)
		req.UpdatedAfter = &ts
	}

	// 2. 调用查询方法
	resp, err := l.calcCpmmTokens(req)
	if err != nil {
		return nil, err
	}

	// 3. 转换结果格式
	items := make([]*trade.CpmmTokenItem, 0, len(resp.Tokens))
	for _, token := range resp.Tokens {
		items = append(items, &trade.CpmmTokenItem{
			Mint:          token.Mint,
			Symbol:        token.Symbol,
			Name:          token.Name,
			Decimals:      token.Decimals,
			Logo:          token.Logo,
			Status:        token.Status,
			ChainId:       token.ChainID,
			Tags:          token.Tags,
			UpdatedAtIso:  token.UpdatedAt.UTC().Format(time.RFC3339),
			UpdatedAtUnix: token.UpdatedAt.Unix(),
		})
	}

	// 4. 返回结果
	return &trade.GetCpmmTokensResponse{
		Tokens:   items,
		PageNo:   uint32(resp.PageNo),
		PageSize: uint32(resp.PageSize),
		Total:    resp.Total,
	}, nil
}

// calcCpmmTokens 查询 CPMM 代币（内部方法）
// 该方法负责：
// 1. 参数标准化
// 2. 调用数据库查询
// 3. 返回分页结果
func (l *GetCpmmTokensLogic) calcCpmmTokens(in *types.GetCpmmTokensInput) (*types.GetCpmmTokensOutput, error) {
	start := time.Now()
	status := "success"

	// 记录查询耗时指标
	defer func() {
		l.metrics.RecordTokenFetch(status, time.Since(start))
	}()

	// 参数标准化
	if in == nil {
		in = &types.GetCpmmTokensInput{}
	}

	// 设置默认链ID
	if in.ChainID == 0 {
		in.ChainID = int64(l.svcCtx.Config.SolConfig.ChainId)
		if in.ChainID == 0 {
			in.ChainID = 100000 // 默认 Solana 主网
		}
	}

	// 设置分页参数
	pageSize := in.PageSize
	if pageSize <= 0 || pageSize > int64(l.svcCtx.LiquidityCfg.TokenPageSize) {
		pageSize = int64(l.svcCtx.LiquidityCfg.TokenPageSize)
	}
	pageNo := in.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}

	// 解析状态列表
	statusList := parseStatuses(in.Statuses)

	// 查询数据库
	items, total, err := l.fetchTokensFromDB(in, statusList, pageNo, pageSize)
	if err != nil {
		status = "error"
		return nil, err
	}

	return &types.GetCpmmTokensOutput{
		Tokens:   items,
		PageNo:   pageNo,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// fetchTokensFromDB 从数据库查询代币列表
func (l *GetCpmmTokensLogic) fetchTokensFromDB(in *types.GetCpmmTokensInput, statuses []string, pageNo, pageSize int64) ([]*types.CpmmToken, int64, error) {
	// 构建查询
	db := l.svcCtx.DB.Table("allowed_tokens").Where("chain_id = ?", in.ChainID)
	if len(statuses) > 0 {
		db = db.Where("status IN ?", statuses)
	}
	if in.UpdatedAfter != nil {
		db = db.Where("updated_at > ?", in.UpdatedAfter.UTC())
	}

	// 统计总数
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计代币数量失败: %w", err)
	}

	// 计算偏移量
	offset := (pageNo - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// 查询分页数据
	var rows []*cpmmAllowedToken
	err := db.Order("mint ASC").
		Offset(int(offset)).
		Limit(int(pageSize)).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询代币失败: %w", err)
	}

	// 转换结果
	items := make([]*types.CpmmToken, 0, len(rows))
	for _, row := range rows {
		items = append(items, convertAllowedToken(row))
	}

	return items, total, nil
}

// cpmmAllowedToken 数据库查询结果结构体
type cpmmAllowedToken struct {
	Mint          string         `gorm:"column:mint"`
	Symbol        string         `gorm:"column:symbol"`
	Name          string         `gorm:"column:name"`
	Decimals      int32          `gorm:"column:decimals"`
	Logo          string         `gorm:"column:logo"`
	Status        string         `gorm:"column:status"`
	ChainID       int64          `gorm:"column:chain_id"`
	Tags          sql.NullString `gorm:"column:tags"`
	PriorityOrder int32          `gorm:"column:priority_order"`
	UpdatedAt     time.Time      `gorm:"column:updated_at"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
}

// convertAllowedToken 转换数据库记录为业务模型
func convertAllowedToken(row *cpmmAllowedToken) *types.CpmmToken {
	return &types.CpmmToken{
		Mint:      row.Mint,
		Symbol:    strings.ToUpper(row.Symbol),
		Name:      row.Name,
		Decimals:  row.Decimals,
		Logo:      row.Logo,
		Status:    row.Status,
		ChainID:   row.ChainID,
		Tags:      parseTags(row.Tags),
		UpdatedAt: row.UpdatedAt,
	}
}

// parseTags 解析标签字段
func parseTags(raw sql.NullString) []string {
	if !raw.Valid {
		return nil
	}
	content := strings.TrimSpace(raw.String)
	if content == "" {
		return nil
	}

	// 尝试 JSON 解析
	var arr []string
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		return arr
	}

	// 尝试逗号分隔解析
	parts := strings.Split(content, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// parseStatuses 解析状态列表
func parseStatuses(raw []string) []string {
	if len(raw) == 0 {
		return []string{"active", "paused", "blocked"}
	}

	// 验证有效状态
	valid := map[string]struct{}{
		"active":  {},
		"paused":  {},
		"blocked": {},
	}

	var out []string
	for _, v := range raw {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			continue
		}
		if _, ok := valid[v]; ok {
			out = append(out, v)
		}
	}

	if len(out) == 0 {
		return []string{"active", "paused", "blocked"}
	}
	return out
}
