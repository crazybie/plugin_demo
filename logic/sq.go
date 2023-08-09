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
	println("new dump 222")
}

//go:noinline
func (sq *Sq) GetId() int {
	return 0
}

//go:noinline
func (sq *Sq) SetId(int) {
}

func (sq *Sq) LoadPatch() {
	patching_sys.ApplyPendingPatch[Sq]()
}
