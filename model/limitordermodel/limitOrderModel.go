package limitordermodel

import (
	"gorm.io/gorm"
)

var _ LimitOrderModel = (*customLimitOrderModel)(nil)

type (
	// LimitOrderModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLimitOrderModel.
	LimitOrderModel interface {
		limitOrderModel
	}

	customLimitOrderModel struct {
		*defaultLimitOrderModel
	}
)

// NewLimitOrderModel returns a model for the database table.
func NewLimitOrderModel(conn *gorm.DB) LimitOrderModel {
	return &customLimitOrderModel{
		defaultLimitOrderModel: newLimitOrderModel(conn),
	}
}
