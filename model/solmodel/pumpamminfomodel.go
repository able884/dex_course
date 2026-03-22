package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ PumpAmmInfoModel = (*customPumpAmmInfoModel)(nil)

type (
	// PumpAmmInfoModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPumpAmmInfoModel.
	PumpAmmInfoModel interface {
		pumpAmmInfoModel
		customPumpAmmInfoLogicModel
	}

	customPumpAmmInfoLogicModel interface {
		WithSession(tx *gorm.DB) PumpAmmInfoModel
	}

	customPumpAmmInfoModel struct {
		*defaultPumpAmmInfoModel
	}
)

func (c customPumpAmmInfoModel) WithSession(tx *gorm.DB) PumpAmmInfoModel {
	newModel := *c.defaultPumpAmmInfoModel
	c.defaultPumpAmmInfoModel = &newModel
	c.conn = tx
	return c
}

// NewPumpAmmInfoModel returns a model for the database table.
func NewPumpAmmInfoModel(conn *gorm.DB) PumpAmmInfoModel {
	return &customPumpAmmInfoModel{
		defaultPumpAmmInfoModel: newPumpAmmInfoModel(conn),
	}
}

func (m *defaultPumpAmmInfoModel) customCacheKeys(data *PumpAmmInfo) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
