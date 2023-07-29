/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package logic

type Sq struct {
	Id int
}

//go:noinline
func (sq *Sq) Dump() {
}

//go:noinline
func (sq *Sq) GetId() int {
	return 0
}

//go:noinline
func (sq *Sq) SetId(int) {
}
