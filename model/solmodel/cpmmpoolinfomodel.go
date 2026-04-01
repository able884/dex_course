package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ CpmmPoolInfoModel = (*customCpmmPoolInfoModel)(nil)

type (
	// CpmmPoolInfoModel is an interface to be customized, add more methods here,
	// and implement the added methods in customCpmmPoolInfoModel.
	CpmmPoolInfoModel interface {
		cpmmPoolInfoModel
		customCpmmPoolInfoLogicModel
	}

	customCpmmPoolInfoLogicModel interface {
		WithSession(tx *gorm.DB) CpmmPoolInfoModel
	}

	customCpmmPoolInfoModel struct {
		*defaultCpmmPoolInfoModel
	}
)

func (c customCpmmPoolInfoModel) WithSession(tx *gorm.DB) CpmmPoolInfoModel {
	newModel := *c.defaultCpmmPoolInfoModel
	c.defaultCpmmPoolInfoModel = &newModel
	c.conn = tx
	return c
}

// NewCpmmPoolInfoModel returns a model for the database table.
func NewCpmmPoolInfoModel(conn *gorm.DB) CpmmPoolInfoModel {
	return &customCpmmPoolInfoModel{
		defaultCpmmPoolInfoModel: newCpmmPoolInfoModel(conn),
	}
}

func (m *defaultCpmmPoolInfoModel) customCacheKeys(data *CpmmPoolInfo) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
