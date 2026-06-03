package limitordermodel

import (
	"gorm.io/gorm"
)

var _ LimitOrderFillModel = (*customLimitOrderFillModel)(nil)

type (
	// LimitOrderFillModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLimitOrderFillModel.
	LimitOrderFillModel interface {
		limitOrderFillModel
	}

	customLimitOrderFillModel struct {
		*defaultLimitOrderFillModel
	}
)

// NewLimitOrderFillModel returns a model for the database table.
func NewLimitOrderFillModel(conn *gorm.DB) LimitOrderFillModel {
	return &customLimitOrderFillModel{
		defaultLimitOrderFillModel: newLimitOrderFillModel(conn),
	}
}
