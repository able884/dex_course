package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ ClmmPoolInfoV1Model = (*customClmmPoolInfoV1Model)(nil)

type (
	// ClmmPoolInfoV1Model is an interface to be customized, add more methods here,
	// and implement the added methods in customClmmPoolInfoV1Model.
	ClmmPoolInfoV1Model interface {
		clmmPoolInfoV1Model
		customClmmPoolInfoV1LogicModel
	}

	customClmmPoolInfoV1LogicModel interface {
		WithSession(tx *gorm.DB) ClmmPoolInfoV1Model
	}

	customClmmPoolInfoV1Model struct {
		*defaultClmmPoolInfoV1Model
	}
)

func (c customClmmPoolInfoV1Model) WithSession(tx *gorm.DB) ClmmPoolInfoV1Model {
	newModel := *c.defaultClmmPoolInfoV1Model
	c.defaultClmmPoolInfoV1Model = &newModel
	c.conn = tx
	return c
}

// NewClmmPoolInfoV1Model returns a model for the database table.
func NewClmmPoolInfoV1Model(conn *gorm.DB) ClmmPoolInfoV1Model {
	return &customClmmPoolInfoV1Model{
		defaultClmmPoolInfoV1Model: newClmmPoolInfoV1Model(conn),
	}
}

func (m *defaultClmmPoolInfoV1Model) customCacheKeys(data *ClmmPoolInfoV1) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
