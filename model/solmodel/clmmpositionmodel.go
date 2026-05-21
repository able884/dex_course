package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ ClmmPositionModel = (*customClmmPositionModel)(nil)

type (
	// ClmmPositionModel is an interface to be customized, add more methods here,
	// and implement the added methods in customClmmPositionModel.
	ClmmPositionModel interface {
		clmmPositionModel
		customClmmPositionLogicModel
	}

	customClmmPositionLogicModel interface {
		WithSession(tx *gorm.DB) ClmmPositionModel
	}

	customClmmPositionModel struct {
		*defaultClmmPositionModel
	}
)

func (c customClmmPositionModel) WithSession(tx *gorm.DB) ClmmPositionModel {
	newModel := *c.defaultClmmPositionModel
	c.defaultClmmPositionModel = &newModel
	c.conn = tx
	return c
}

// NewClmmPositionModel returns a model for the database table.
func NewClmmPositionModel(conn *gorm.DB) ClmmPositionModel {
	return &customClmmPositionModel{
		defaultClmmPositionModel: newClmmPositionModel(conn),
	}
}

func (m *defaultClmmPositionModel) customCacheKeys(data *ClmmPosition) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}

