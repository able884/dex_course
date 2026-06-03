package limitordermodel

import (
	"gorm.io/gorm"
)

var _ LimitOrderMarginModel = (*customLimitOrderMarginModel)(nil)

type (
	// LimitOrderMarginModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLimitOrderMarginModel.
	LimitOrderMarginModel interface {
		limitOrderMarginModel
	}

	customLimitOrderMarginModel struct {
		*defaultLimitOrderMarginModel
	}
)

// NewLimitOrderMarginModel returns a model for the database table.
func NewLimitOrderMarginModel(conn *gorm.DB) LimitOrderMarginModel {
	return &customLimitOrderMarginModel{
		defaultLimitOrderMarginModel: newLimitOrderMarginModel(conn),
	}
}
