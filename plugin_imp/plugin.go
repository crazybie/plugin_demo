package plugin_imp

import (
	"plugin_demo/opmap"
)

type Sq struct {
	opmap.Sq
}

func (sq *Sq) Dump() {
	println("id", sq.GetId())
}

func (sq *Sq) GetId() int {
	return sq.Id
}

func (sq *Sq) SetId(v int) {
	sq.Id = v
}

func NewSq() any {
	return (*Sq)(nil)
}
