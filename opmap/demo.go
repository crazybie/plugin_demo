package opmap

import "plugin_demo/plugin_sys"

type Sq struct {
	Id int
}

var SqClass, sqImp = plugin_sys.NewPatchable(struct {
	Dump  func(*Sq)
	GetId func(*Sq) int
	SetId func(*Sq, int)
}{})

func (sq *Sq) Dump() {
	sqImp.Dump(sq)
}

func (sq *Sq) GetId() int {
	return sqImp.GetId(sq)
}

func (sq *Sq) SetId(v int) {
	sqImp.SetId(sq, v)
}
