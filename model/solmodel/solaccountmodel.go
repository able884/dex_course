package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ SolAccountModel = (*customSolAccountModel)(nil)

type (
	// SolAccountModel is an interface to be customized, add more methods here,
	// and implement the added methods in customSolAccountModel.
	SolAccountModel interface {
		solAccountModel
		customSolAccountLogicModel
	}

	customSolAccountLogicModel interface {
		WithSession(tx *gorm.DB) SolAccountModel
	}

	customSolAccountModel struct {
		*defaultSolAccountModel
	}
)

func (c customSolAccountModel) WithSession(tx *gorm.DB) SolAccountModel {
	newModel := *c.defaultSolAccountModel
	c.defaultSolAccountModel = &newModel
	c.conn = tx
	return c
}

// NewSolAccountModel returns a model for the database table.
func NewSolAccountModel(conn *gorm.DB) SolAccountModel {
	return &customSolAccountModel{
		defaultSolAccountModel: newSolAccountModel(conn),
	}
}

func (m *defaultSolAccountModel) customCacheKeys(data *SolAccount) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
