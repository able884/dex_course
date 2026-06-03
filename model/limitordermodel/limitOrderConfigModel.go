package limitordermodel

import (
	"gorm.io/gorm"
)

var _ LimitOrderConfigModel = (*customLimitOrderConfigModel)(nil)

type (
	// LimitOrderConfigModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLimitOrderConfigModel.
	LimitOrderConfigModel interface {
		limitOrderConfigModel
	}

	customLimitOrderConfigModel struct {
		*defaultLimitOrderConfigModel
	}
)

// NewLimitOrderConfigModel returns a model for the database table.
func NewLimitOrderConfigModel(conn *gorm.DB) LimitOrderConfigModel {
	return &customLimitOrderConfigModel{
		defaultLimitOrderConfigModel: newLimitOrderConfigModel(conn),
	}
}
