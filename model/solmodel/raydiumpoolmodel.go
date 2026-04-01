package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ RaydiumPoolModel = (*customRaydiumPoolModel)(nil)

type (
	// RaydiumPoolModel is an interface to be customized, add more methods here,
	// and implement the added methods in customRaydiumPoolModel.
	RaydiumPoolModel interface {
		raydiumPoolModel
		customRaydiumPoolLogicModel
	}

	customRaydiumPoolLogicModel interface {
		WithSession(tx *gorm.DB) RaydiumPoolModel
	}

	customRaydiumPoolModel struct {
		*defaultRaydiumPoolModel
	}
)

func (c customRaydiumPoolModel) WithSession(tx *gorm.DB) RaydiumPoolModel {
	newModel := *c.defaultRaydiumPoolModel
	c.defaultRaydiumPoolModel = &newModel
	c.conn = tx
	return c
}

// NewRaydiumPoolModel returns a model for the database table.
func NewRaydiumPoolModel(conn *gorm.DB) RaydiumPoolModel {
	return &customRaydiumPoolModel{
		defaultRaydiumPoolModel: newRaydiumPoolModel(conn),
	}
}

func (m *defaultRaydiumPoolModel) customCacheKeys(data *RaydiumPool) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
