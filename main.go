package main

import (
	"plugin_demo/opmap"
	"plugin_demo/plugin_imp"
)

func main() {
	sq := opmap.Sq{Id: 1}

	opmap.SqClass.Patch(plugin_imp.NewSq())

	sq.Dump()
	sq.SetId(111)
	println(sq.GetId())
}
