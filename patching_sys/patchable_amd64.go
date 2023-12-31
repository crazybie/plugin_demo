/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package patching_sys

func codeGenJmpTo(to uintptr) []byte {
	return []byte{
		0x48, 0xBA,
		byte(to),
		byte(to >> 8),
		byte(to >> 16),
		byte(to >> 24),
		byte(to >> 32),
		byte(to >> 40),
		byte(to >> 48),
		byte(to >> 56), // movabs rdx, to
		0xFF, 0xE2,     // jmp rdx
	}
}
