package game

import (
	"github.com/zond/diplicity/common"
	"github.com/zond/kcwraps/kol"
)

type Minutes int

type Game struct {
	Id      []byte
	Closed  bool `kol:"index"`
	Started bool `kol:"index"`
	Variant string
	EndYear int
	Private bool `kol:"index"`
	Owner   []byte

	Deadlines map[string]Minutes

	ChatFlags map[string]common.ChatFlag
}

func Open() *kol.Query {
	return common.DB.Query().Filter(kol.Equals{"Closed", false})
}