package plugin_imp

import (
	"plugin_demo/logic"
)

type Sq struct {
	logic.Sq
}

func (sq *Sq) Dump() {
	println("id", sq.GetId())
}

func (sq *Sq) GetId() int {
	return sq.Id
}

func (sq *Sq) SetId(v int) {
	println("set id to", v)
	sq.Id = v
}

func NewSq() any {
	return (*Sq)(nil)
}
