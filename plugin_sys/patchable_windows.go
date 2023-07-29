package plugin_sys

import (
	"syscall"
	"unsafe"
)

const flagsPageExecuteReadwrite = 0x40

var impMProtect = syscall.NewLazyDLL("kernel32.dll").NewProc("VirtualProtect")

func mProtect(addr uintptr, size int, flags uint32) (outFlags uint32) {
	r, _, _ := impMProtect.Call(addr, uintptr(size), uintptr(flags), uintptr(unsafe.Pointer(&outFlags)))
	if r == 0 {
		panic(syscall.GetLastError())
	}
	return
}

func execMemCopy(addr uintptr, data []byte) {
	oldFlags := mProtect(addr, len(data), flagsPageExecuteReadwrite)
	copy(addrAsBytes(addr, len(data)), data[:])
	mProtect(addr, len(data), oldFlags)
}