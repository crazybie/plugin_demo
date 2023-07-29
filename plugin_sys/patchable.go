/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package plugin_sys

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

type patchedMethod struct {
	orig  reflect.Value
	patch reflect.Value
}

func Patch[OrigTp any](patchObjPtr any) {
	origPtrTp := reflect.TypeOf((*OrigTp)(nil))
	patchPtrTp := reflect.TypeOf(patchObjPtr)
	for _, p := range checkPatchTp(origPtrTp, patchPtrTp) {
		patchFn(p.orig, p.patch)
	}
}

func checkPatchTp(origPtrTp, patchPtrTp reflect.Type) (patchMethods []patchedMethod) {
	patch := patchPtrTp.Elem()
	orig := origPtrTp.Elem()
	embeddedTp := patch.Field(0).Type

	if patch.Size() != orig.Size() {
		panic(fmt.Errorf("patch type must not have extra fields, %s", patch))
	}
	if embeddedTp != orig {
		panic(fmt.Errorf("patch type must embed orig type as first elem, orig: %s, patch: %s, first elem: %s", orig, patch, embeddedTp))
	}

	for i := 0; i < origPtrTp.NumMethod(); i++ {
		origFn := origPtrTp.Method(i)
		newFn, ok := patchPtrTp.MethodByName(origFn.Name)
		if ok && !isPromotedMethod(newFn.Func) {
			if err := checkFnSig(origFn.Type, newFn.Func.Type()); err != nil {
				panic(fmt.Errorf("%s method %s, %w", origPtrTp, origFn.Name, err))
			}
			patchMethods = append(patchMethods, patchedMethod{origFn.Func, newFn.Func})
		}
	}
	return
}

func checkFnSig(to, from reflect.Type) error {
	if from.NumIn() != to.NumIn() {
		return fmt.Errorf("input param count mismatch, orig: %v, patch: %v", to, from)
	}
	if from.NumOut() != to.NumOut() {
		return fmt.Errorf("output param count mismatch, orig: %v, patch: %v", to, from)
	}
	for i := 0; i < from.NumIn(); i++ {
		patch := from.In(i)
		orig := to.In(i)
		if i == 0 {
			if patch.Elem().Field(0).Type != orig.Elem() {
				return fmt.Errorf("receiver not compatible, orig: %s, patch: %s", orig, patch)
			}
		} else if patch != orig {
			return fmt.Errorf("input param type mismatch, orig: %s, patch: %s", orig, patch)
		}
	}
	for i := 0; i < from.NumOut(); i++ {
		orig := to.Out(i)
		patch := from.Out(i)
		if orig != patch {
			return fmt.Errorf("output param type mismatch, orig: %s, path: %s", orig, patch)
		}
	}
	return nil
}

func isPromotedMethod(f reflect.Value) bool {
	if f.Kind() != reflect.Func {
		return false
	}
	fn := runtime.FuncForPC(f.Pointer())
	fName, _ := fn.FileLine(fn.Entry())
	return fName == "<autogenerated>"
}

type inner struct{}

type outer struct {
	inner
}

func (*inner) Foo() {}

//go:noinline
func a1() int { return 1 }

//go:noinline
func a2() int { return 2 }

func init() {
	if !isPromotedMethod(reflect.TypeOf((*outer)(nil)).Method(0).Func) {
		panic("isPromotedMethod not work")
	}

	patchFn(reflect.ValueOf(a1), reflect.ValueOf(a2))
	if a1() != 2 {
		panic("patching not work for this platform")
	}
}

func getFnCodePtr(v reflect.Value) uintptr {
	return (*[2]uintptr)(unsafe.Pointer(&v))[1]
}

func patchFn(orig, patch reflect.Value) {
	execMemCopy(orig.Pointer(), codeGenJmpTo(getFnCodePtr(patch)))
}

func bytes(addr uintptr, size int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
}
