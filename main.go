package main

import (
	"plugin_demo/logic"
	"plugin_demo/plugin_imp"
	"plugin_demo/plugin_sys"
)

func main() {
	sq := logic.Sq{Id: 1}

	plugin_sys.Patch[logic.Sq](plugin_imp.NewSq())

	sq.Dump()
	sq.SetId(111)
	println(sq.GetId())
}
