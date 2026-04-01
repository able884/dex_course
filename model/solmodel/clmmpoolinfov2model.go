package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ ClmmPoolInfoV2Model = (*customClmmPoolInfoV2Model)(nil)

type (
	// ClmmPoolInfoV2Model is an interface to be customized, add more methods here,
	// and implement the added methods in customClmmPoolInfoV2Model.
	ClmmPoolInfoV2Model interface {
		clmmPoolInfoV2Model
		customClmmPoolInfoV2LogicModel
	}

	customClmmPoolInfoV2LogicModel interface {
		WithSession(tx *gorm.DB) ClmmPoolInfoV2Model
	}

	customClmmPoolInfoV2Model struct {
		*defaultClmmPoolInfoV2Model
	}
)

func (c customClmmPoolInfoV2Model) WithSession(tx *gorm.DB) ClmmPoolInfoV2Model {
	newModel := *c.defaultClmmPoolInfoV2Model
	c.defaultClmmPoolInfoV2Model = &newModel
	c.conn = tx
	return c
}

// NewClmmPoolInfoV2Model returns a model for the database table.
func NewClmmPoolInfoV2Model(conn *gorm.DB) ClmmPoolInfoV2Model {
	return &customClmmPoolInfoV2Model{
		defaultClmmPoolInfoV2Model: newClmmPoolInfoV2Model(conn),
	}
}

func (m *defaultClmmPoolInfoV2Model) customCacheKeys(data *ClmmPoolInfoV2) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
