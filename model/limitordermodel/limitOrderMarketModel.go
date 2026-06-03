package limitordermodel

import (
	"gorm.io/gorm"
)

var _ LimitOrderMarketModel = (*customLimitOrderMarketModel)(nil)

type (
	// LimitOrderMarketModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLimitOrderMarketModel.
	LimitOrderMarketModel interface {
		limitOrderMarketModel
	}

	customLimitOrderMarketModel struct {
		*defaultLimitOrderMarketModel
	}
)

// NewLimitOrderMarketModel returns a model for the database table.
func NewLimitOrderMarketModel(conn *gorm.DB) LimitOrderMarketModel {
	return &customLimitOrderMarketModel{
		defaultLimitOrderMarketModel: newLimitOrderMarketModel(conn),
	}
}
