//go:build (linux && cgo) || (darwin && cgo) || (freebsd && cgo)

/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package patching_sys

import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unsafe"
)

/*
   #cgo linux LDFLAGS: -ldl
   #include <dlfcn.h>
   #include <limits.h>
   #include <stdlib.h>
   #include <stdint.h>
   #include <stdio.h>

   static void* pluginOpen(const char* path, char** err) {
   	void* h = dlopen(path, RTLD_NOW|RTLD_GLOBAL);
   	if (h == NULL) {
   		*err = (char*)dlerror();
   	}
   	return h;
   }
*/
import "C"

var UnsafeMode = true

func init() {
	startDaemon()
}

func typeToKey(v reflect.Type) string {
	return v.PkgPath() + "." + v.String()
}

var tpNameReg = regexp.MustCompile(`\.main\.SoTmp\d+_`)

func convertToOrigTypeKey(c string) string {
	items := strings.Split(c, "/")
	newPkg := strings.Split(items[len(items)-1], ".")[0]

	// remove version
	pkg := regexp.MustCompile(`_\d+$`).ReplaceAllString(newPkg, "")
	c = strings.ReplaceAll(c, newPkg, pkg)

	return tpNameReg.ReplaceAllString(c, fmt.Sprintf(".%s.", pkg))
}

type Factory interface {
	GetTypes() map[string]reflect.Type
	Init() string
	RegisterTp(t any)
	Reset()
}

type SoTypeFactory struct {
	types map[string]reflect.Type
}

func (s *SoTypeFactory) RegisterTp(t any) {
	v := reflect.TypeOf(t)
	k := typeToKey(v.Elem())
	if _, ok := s.types[k]; !ok {
		// fmt.Printf("register type: %s\n", v.String())
		s.types[k] = v
	}
}

func (s *SoTypeFactory) GetTypes() map[string]reflect.Type {
	return s.types
}

func (s *SoTypeFactory) Init() string {
	return "default"
}

func (s *SoTypeFactory) Reset() {
	s.types = map[string]reflect.Type{}
}

var defaultFactory = SoTypeFactory{
	types: map[string]reflect.Type{},
}

var pendingTypes = struct {
	sync.RWMutex
	types map[string]reflect.Type
}{types: map[string]reflect.Type{}}

func Register[T any]() {
	defaultFactory.RegisterTp((*T)(nil))
}

func HasPatch[T any]() bool {
	pendingTypes.RLock()
	defer pendingTypes.RUnlock()

	old := reflect.TypeOf((*T)(nil))
	tpKey := typeToKey(old.Elem())
	_, ok := pendingTypes.types[tpKey]
	return ok
}

func ApplyPatch[T any]() {
	pendingTypes.Lock()
	defer pendingTypes.Unlock()

	old := reflect.TypeOf((*T)(nil))
	tpKey := typeToKey(old.Elem())
	tp, ok := pendingTypes.types[tpKey]
	if !ok {
		return
	}
	defer delete(pendingTypes.types, tpKey)
	patchType(old, tp)
}

func patchType(old, tp reflect.Type) bool {
	fmt.Printf("apply patching to %s\n", old.String())
	fns := scanMethods(old, tp)
	for _, p := range fns {
		patchFn(p.orig, p.patch)
	}
	fmt.Printf("patched methods: %v\n", len(fns))
	return len(fns) > 0
}

func scanTypesFromPlugin(so string) bool {
	bypassPkgHashCheck(so)
	p, err := plugin.Open(so)
	if err != nil {
		panic(fmt.Errorf("so loading failed, %s\n", err))
	}
	defer unload(p)

	fc, err := p.Lookup("GetFactory")
	if err != nil {
		panic(fmt.Errorf("entry point not found in plugin: %s", so))
	}
	f := fc.(func() Factory)()

	ver := f.Init()
	if !strings.HasSuffix(so, ver+".so") {
		panic(fmt.Errorf("so version err: %s, runtime: %s", so, ver))
	}

	newTps := f.GetTypes()
	patched := false
	for c, tp := range newTps {
		validateTpVer(tp, ver)
		k := convertToOrigTypeKey(c)

		if UnsafeMode {
			old, ok := defaultFactory.types[k]
			if ok {
				patched = patchType(old, tp)
			}
		} else {
			pendingTypes.Lock()
			pendingTypes.types[k] = tp
			pendingTypes.Unlock()
			fmt.Printf("patch detected: %s\n", k)
		}
	}

	return patched
}

func validateTpVer(tp reflect.Type, ver string) {
	validTp := false
	if fnVer, ok := tp.MethodByName("Ver__"); ok {
		if ret := fnVer.Func.Call([]reflect.Value{reflect.New(tp.Elem())}); ret != nil {
			validTp = ret[0].String() == ver
		}
	}
	if validTp {
		fmt.Printf("version validation succeeded\n")
	} else {
		panic("patching not work, new method not generated correctly.")
	}
}

func startDaemon() {
	fmt.Printf("plugin daemon started.\n")

	soDir, _ := os.Getwd()
	if t := os.Getenv("SO_DIR"); t != "" {
		soDir = t
	}

	go func() {
		for {
			func() {
				defer func() {
					if err := recover(); err != nil {
						fmt.Printf("panic: %s\n%s", err, debug.Stack())
					}
				}()

				handleSo(soDir)
				time.Sleep(time.Second)
			}()
		}
	}()
}

func handleSo(soDir string) {
	items, _ := os.ReadDir(soDir)
	for _, item := range items {
		if strings.HasSuffix(item.Name(), ".so") {

			full := filepath.Join(soDir, item.Name())
			fmt.Printf("detected new so: %s\n", item.Name())
			if scanTypesFromPlugin(full) {
				_ = os.Remove(full)
			}
		}
	}
}

type patchingMethod struct {
	orig  reflect.Value
	patch reflect.Value
}

func Patch[OrigTp any](patchObjPtr any) {
	origPtrTp := reflect.TypeOf((*OrigTp)(nil))
	patchPtrTp := reflect.TypeOf(patchObjPtr)
	for _, p := range scanMethods(origPtrTp, patchPtrTp) {
		patchFn(p.orig, p.patch)
	}
}

func scanMethods(origPtrTp, patchPtrTp reflect.Type) (methods []patchingMethod) {
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
		if ok {
			if err := checkFnSig(origFn.Type, newFn.Func.Type()); err != nil {
				panic(fmt.Errorf("%s method %s, %w", origPtrTp, origFn.Name, err))
			}
			methods = append(methods, patchingMethod{origFn.Func, newFn.Func})
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

//============================================================================
// Utils
//============================================================================

//go:noinline
func a1() int { return 1 }

//go:noinline
func a2() int { return 2 }

func init() {
	patchFn(reflect.ValueOf(a1), reflect.ValueOf(a2))
	if a1() != 2 {
		panic("patching not work for this platform")
	}
}

func patchFn(orig, patch reflect.Value) {
	if orig == patch || orig.Pointer() == patch.Pointer() {
		panic("same function")
	}
	execMemCopy(orig.Pointer(), codeGenJmpTo(patch.Pointer()))
}

func bytes(addr uintptr, size int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
}

//============================================================================
// HACK
//============================================================================

// copy from runtime/plugin.go

//go:linkname firstmoduledata runtime.firstmoduledata
var firstmoduledata moduledata

type modulehash struct {
	modulename   string
	linktimehash string
	runtimehash  *string
}

type textsect struct {
	vaddr    uintptr // prelinked section vaddr
	end      uintptr // vaddr + section length
	baseaddr uintptr // relocated section address
}

type functab struct {
	entryoff uint32 // relative to runtime.text
	funcoff  uint32
}

// A ptabEntry is generated by the compiler for each exported function
// and global variable in the main package of a plugin. It is used to
// initialize the plugin module's symbol map.
type ptabEntry struct {
	name uint32
	typ  uint32
}

type bitvector struct {
	n        int32 // # of bits
	bytedata *uint8
}

type moduledata struct {
	pcHeader     uintptr
	funcnametab  []byte
	cutab        []uint32
	filetab      []byte
	pctab        []byte
	pclntable    []byte
	ftab         []functab
	findfunctab  uintptr
	minpc, maxpc uintptr

	text, etext           uintptr
	noptrdata, enoptrdata uintptr
	data, edata           uintptr
	bss, ebss             uintptr
	noptrbss, enoptrbss   uintptr
	covctrs, ecovctrs     uintptr
	end, gcdata, gcbss    uintptr
	types, etypes         uintptr
	rodata                uintptr
	gofunc                uintptr // go.func.*

	textsectmap []textsect
	typelinks   []int32 // offsets from types
	itablinks   []uintptr

	ptab []ptabEntry

	pluginpath string
	pkghashes  []modulehash

	modulename   string
	modulehashes []modulehash

	hasmain uint8 // 1 if module contains the main function, 0 otherwise

	gcdatamask, gcbssmask bitvector

	typemap map[uintptr]uintptr // offset to *_rtype in previous module

	bad bool // module failed to load and should be ignored

	next *moduledata
}

// Plugin is a loaded Go plugin.
type Plugin struct {
	pluginpath string
	err        string        // set if plugin failed to load
	loaded     chan struct{} // closed when loaded
}

func bypassPkgHashCheck(name string) {
	cPath := make([]byte, C.PATH_MAX+1)
	cRelName := make([]byte, len(name)+1)
	copy(cRelName, name)
	if C.realpath(
		(*C.char)(unsafe.Pointer(&cRelName[0])),
		(*C.char)(unsafe.Pointer(&cPath[0]))) == nil {
		return
	}

	var cErr *C.char
	C.pluginOpen((*C.char)(unsafe.Pointer(&cPath[0])), &cErr)
	if cErr != nil {
		return
	}
	md := firstmoduledata.next
	for pmd := firstmoduledata.next; pmd != nil; pmd = pmd.next {
		if pmd.bad {
			md = nil // we only want the last module
			continue
		}
		md = pmd
	}
	newhash := make([]modulehash, 0, len(md.pkghashes))
	for _, pkghash := range md.pkghashes {
		pkghash.linktimehash = *pkghash.runtimehash
		newhash = append(newhash, pkghash)
	}
	md.pkghashes = newhash
}

func unload(p *plugin.Plugin) {
	imp := (*Plugin)(unsafe.Pointer(p))
	md := firstmoduledata.next
	for pmd := firstmoduledata.next; pmd != nil; pmd = pmd.next {
		if pmd.bad {
			md = nil // we only want the last module
			continue
		}
		md = pmd
		if md.pluginpath == imp.pluginpath {
			md.pluginpath = ""
			return
		}
	}
}
