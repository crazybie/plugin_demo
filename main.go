package main

import (
	"plugin_demo/logic"
	"plugin_demo/plugin_imp"
)

func main() {
	sq := logic.Sq{Id: 1}

	logic.SqClass.Patch(plugin_imp.NewSq())

	sq.Dump()
	sq.SetId(111)
	println(sq.GetId())
}
