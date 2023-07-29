//go:build !windows

/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package plugin_sys

import (
	"syscall"
)

const (
	flagsPageReadwriteExecute = syscall.PROT_READ | syscall.PROT_WRITE | syscall.PROT_EXEC
	flagsPageReadExecute      = syscall.PROT_READ | syscall.PROT_EXEC
)

func mProtect(addr uintptr, size int, flags int) {
	pageSize := syscall.Getpagesize()
	pageStart := addr & ^(uintptr(pageSize - 1))
	for p := pageStart; p < addr+uintptr(size); p += uintptr(pageSize) {
		err := syscall.Mprotect(addrAsBytes(p, pageSize), flags)
		if err != nil {
			panic(err)
		}
	}
}

func execMemCopy(addr uintptr, data []byte) {
	mProtect(addr, len(data), flagsPageReadwriteExecute)
	copy(addrAsBytes(addr, len(data)), data[:])
	mProtect(addr, len(data), flagsPageReadExecute)
}
