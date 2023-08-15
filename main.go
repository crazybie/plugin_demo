/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package main

import (
	"plugin_demo/logic"
	"reflect"
	"time"
)

func useSq(sq *logic.Sq) {
	sq.Dump()
}

func main() {
	sq := logic.Sq{Id: 1}

	for {
		time.Sleep(5 * time.Second)
		sq.LoadPatch()

		sq.Dump()
		sq.SetId(111)
		println(sq.GetId())

		reflect.ValueOf(useSq).Call([]reflect.Value{reflect.ValueOf(sq.Clone())})
	}

}
