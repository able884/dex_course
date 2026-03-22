package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ PairModel = (*customPairModel)(nil)

type (
	// PairModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPairModel.
	PairModel interface {
		pairModel
		customPairLogicModel
	}

	customPairLogicModel interface {
		WithSession(tx *gorm.DB) PairModel
	}

	customPairModel struct {
		*defaultPairModel
	}
)

func (c customPairModel) WithSession(tx *gorm.DB) PairModel {
	newModel := *c.defaultPairModel
	c.defaultPairModel = &newModel
	c.conn = tx
	return c
}

// NewPairModel returns a model for the database table.
func NewPairModel(conn *gorm.DB) PairModel {
	return &customPairModel{
		defaultPairModel: newPairModel(conn),
	}
}

func (m *defaultPairModel) customCacheKeys(data *Pair) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
