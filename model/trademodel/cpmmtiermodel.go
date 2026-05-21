package trademodel

import "gorm.io/gorm"

var _ CpmmFeeTierModel = (*customCpmmFeeTierModel)(nil)

type (
	// CpmmFeeTierModel can be customized with extra methods.
	CpmmFeeTierModel interface {
		cpmmFeeTierModel
		WithSession(tx *gorm.DB) CpmmFeeTierModel
	}

	customCpmmFeeTierModel struct {
		*defaultCpmmFeeTierModel
	}
)

func (c customCpmmFeeTierModel) WithSession(tx *gorm.DB) CpmmFeeTierModel {
	newModel := *c.defaultCpmmFeeTierModel
	c.defaultCpmmFeeTierModel = &newModel
	c.conn = tx
	return c
}

// NewCpmmFeeTierModel returns a model for the cpmm_fee_tiers table.
func NewCpmmFeeTierModel(conn *gorm.DB) CpmmFeeTierModel {
	return &customCpmmFeeTierModel{
		defaultCpmmFeeTierModel: newCpmmFeeTierModel(conn),
	}
}
