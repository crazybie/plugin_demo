/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package logic

import "plugin_demo/patching_sys"

type Sq struct {
	Id int
}

//go:noinline
func (sq *Sq) Dump() {
	println("new dump", sq.Id)
}

//go:noinline
func (sq *Sq) GetId() int {
	return sq.Id
}

//go:noinline
func (sq *Sq) SetId(v int) {
	sq.Id = v
}

func (sq *Sq) LoadPatch() {
	patching_sys.ApplyPendingPatch[Sq]()
}

func (sq *Sq) Clone() *Sq {
	return &Sq{1222}
}
