package solmodel

import (
	. "github.com/klen-ygs/gorm-zero/gormc/sql"
	"gorm.io/gorm"
)

// avoid unused err
var _ = InitField
var _ TokenModel = (*customTokenModel)(nil)

type (
	// TokenModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTokenModel.
	TokenModel interface {
		tokenModel
		customTokenLogicModel
	}

	customTokenLogicModel interface {
		WithSession(tx *gorm.DB) TokenModel
	}

	customTokenModel struct {
		*defaultTokenModel
	}
)

func (c customTokenModel) WithSession(tx *gorm.DB) TokenModel {
	newModel := *c.defaultTokenModel
	c.defaultTokenModel = &newModel
	c.conn = tx
	return c
}

// NewTokenModel returns a model for the database table.
func NewTokenModel(conn *gorm.DB) TokenModel {
	return &customTokenModel{
		defaultTokenModel: newTokenModel(conn),
	}
}

func (m *defaultTokenModel) customCacheKeys(data *Token) []string {
	if data == nil {
		return []string{}
	}
	return []string{}
}
