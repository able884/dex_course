package solmodel

import (
	"context"

	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
	"richcode.cc/dex/pkg/xcode"
)

// avoid unused err
var _ = InitField
var _ PairModel = (*customPairModel)(nil)

type (
	// PairModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPairModel.
	PairModel interface {
		pairModel
		customPairLogicModel
	}

	customPairLogicModel interface {
		WithSession(tx *gorm.DB) PairModel
		FindOneByChainIdTokenAddress(ctx context.Context, chainId int64, tokenAddress string) (*Pair, error)
		FindLatestPumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error)
		FindLatestCompletingPumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error)
		FindLatestCompletePumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error)
	}

	customPairModel struct {
		*defaultPairModel
	}
)

func (c customPairModel) WithSession(tx *gorm.DB) PairModel {
	newModel := *c.defaultPairModel
	c.defaultPairModel = &newModel
	c.conn = tx
	return c
}

// NewPairModel returns a model for the database table.
func NewPairModel(conn *gorm.DB) PairModel {
	return &customPairModel{
		defaultPairModel: newPairModel(conn),
	}
}

func (m *defaultPairModel) customCacheKeys(data *Pair) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

func (m customPairModel) FindOneByChainIdTokenAddress(ctx context.Context, chainId int64, tokenAddress string) (*Pair, error) {
	var res []Pair
	err := m.conn.WithContext(ctx).Model(&Pair{}).Where("chain_id = ? and token_address = ?", chainId, tokenAddress).Find(&res).Error
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, xcode.NotingFoundError
	}

	categoryMap := make(map[string][]Pair)
	for _, pair := range res {
		categoryMap[pair.Name] = append(categoryMap[pair.Name], pair)
	}

	temporaryRes := make([]Pair, 0, len(categoryMap))
	for _, pairs := range categoryMap {
		var (
			liquidityPair Pair
			selected      bool
			maxLiquidity  float64
		)
		for _, pair := range pairs {
			if !selected || pair.Liquidity > maxLiquidity {
				maxLiquidity = pair.Liquidity
				liquidityPair = pair
				selected = true
			}
		}
		if selected {
			temporaryRes = append(temporaryRes, liquidityPair)
		}
	}

	if len(temporaryRes) == 0 {
		temporaryRes = res
	}

	var (
		contractResult Pair
		selected       bool
		maxLiquidity   float64
	)
	for _, pair := range temporaryRes {
		if !selected || pair.Liquidity > maxLiquidity {
			maxLiquidity = pair.Liquidity
			contractResult = pair
			selected = true
		}
	}

	if !selected {
		return nil, xcode.NotingFoundError
	}

	return &contractResult, nil
}

func (m *customPairModel) buildPumpQuery(ctx context.Context, pageNum, pageSize int32) (*gorm.DB, int) {
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	offset := int((pageNum - 1) * pageSize)
	query := m.conn.WithContext(ctx).Model(&Pair{})

	return query, offset
}

// FindLatestPumpLimit 查询最新创建的 Pump 代币
func (m customPairModel) FindLatestPumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error) {
	query, offset := m.buildPumpQuery(ctx, pageNum, pageSize)

	resp := make([]Pair, 0, pageSize)
	err := query.
		Where("name = ?", pumpType).
		Order("block_num DESC").
		Offset(offset).
		Limit(int(pageSize)).
		Find(&resp).Error

	return resp, err
}

// FindLatestCompletingPumpLimit 查询正在完成中的 Pump 代币
func (m customPairModel) FindLatestCompletingPumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error) {
	query, offset := m.buildPumpQuery(ctx, pageNum, pageSize)

	resp := make([]Pair, 0, pageSize)
	err := query.
		Where("name = ? AND pump_status = ?", pumpType, 1).
		Order("pump_point DESC").
		Offset(offset).
		Limit(int(pageSize)).
		Find(&resp).Error

	return resp, err
}

// FindLatestCompletePumpLimit 查询已完成的 Pump 代币
func (m customPairModel) FindLatestCompletePumpLimit(ctx context.Context, pumpType string, pageNum, pageSize int32) ([]Pair, error) {
	query, offset := m.buildPumpQuery(ctx, pageNum, pageSize)

	resp := make([]Pair, 0, pageSize)
	err := query.
		Where("name = ? AND pump_status = ?", pumpType, 2).
		Order("block_num DESC").
		Offset(offset).
		Limit(int(pageSize)).
		Find(&resp).Error

	return resp, err
}
