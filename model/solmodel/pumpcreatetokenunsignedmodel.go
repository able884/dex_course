package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ PumpCreateTokenUnsignedModel = (*customPumpCreateTokenUnsignedModel)(nil)

type (
	// PumpCreateTokenUnsignedModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPumpCreateTokenUnsignedModel.
	PumpCreateTokenUnsignedModel interface {
		pumpCreateTokenUnsignedModel
		customPumpCreateTokenUnsignedLogicModel
	}

	customPumpCreateTokenUnsignedLogicModel interface {
		WithSession(tx *gorm.DB) PumpCreateTokenUnsignedModel
	}

	customPumpCreateTokenUnsignedModel struct {
		*defaultPumpCreateTokenUnsignedModel
	}
)

func (c customPumpCreateTokenUnsignedModel) WithSession(tx *gorm.DB) PumpCreateTokenUnsignedModel {
	newModel := *c.defaultPumpCreateTokenUnsignedModel
	c.defaultPumpCreateTokenUnsignedModel = &newModel
	c.conn = tx
	return c
}

// NewPumpCreateTokenUnsignedModel returns a model for the database table.
func NewPumpCreateTokenUnsignedModel(conn *gorm.DB) PumpCreateTokenUnsignedModel {
	return &customPumpCreateTokenUnsignedModel{
		defaultPumpCreateTokenUnsignedModel: newPumpCreateTokenUnsignedModel(conn),
	}
}

func (m *defaultPumpCreateTokenUnsignedModel) customCacheKeys(data *PumpCreateTokenUnsigned) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
