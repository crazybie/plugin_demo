//go:build !windows

package plugin_sys

import (
	"syscall"
)

const (
	flagsPageExecuteReadwrite = syscall.PROT_READ | syscall.PROT_WRITE | syscall.PROT_EXEC
	flagsPageExecuteRead      = syscall.PROT_READ | syscall.PROT_EXEC
)

var pageSize int

func init() {
	pageSize = syscall.Getpagesize()
}

func mProtect(addr uintptr, size int, flags int) {
	pageStart := addr & ^(uintptr(pageSize - 1))
	for p := pageStart; p < addr+uintptr(size); p += uintptr(pageSize) {
		err := syscall.Mprotect(addrAsBytes(p, pageSize), flags)
		if err != nil {
			panic(err)
		}
	}
}

func execMemCopy(addr uintptr, data []byte) {
	mProtect(addr, len(data), flagsPageExecuteReadwrite)
	copy(addrAsBytes(addr, len(data)), data[:])
	mProtect(addr, len(data), flagsPageExecuteRead)
}
