/*
 * Copyright (C) 2023 crazybie@github.com.
 *
 */

package patching_sys

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

type SoFactory struct {
	types map[string]reflect.Type
}

func (s *SoFactory) RegisterTp(t any) {
	v := reflect.TypeOf(t)
	k := typeToKey(v.Elem())
	if _, ok := s.types[k]; !ok {
		// fmt.Printf("register type: %s\n", v.String())
		s.types[k] = v
	}
}

func (s *SoFactory) GetTypes() map[string]reflect.Type {
	return s.types
}

func (s *SoFactory) Init() string {
	return "default"
}

func (s *SoFactory) Reset() {
	s.types = map[string]reflect.Type{}
}

var defaultFactory = SoFactory{
	types: map[string]reflect.Type{},
}

var pendingTypes = struct {
	sync.RWMutex
	types map[string]reflect.Type
}{types: map[string]reflect.Type{}}

func ApplyPendingPatch[T any]() {
	defaultFactory.RegisterTp((*T)(nil))

	pendingTypes.Lock()
	defer pendingTypes.Unlock()

	old := reflect.TypeOf((*T)(nil))
	tpKey := typeToKey(old.Elem())
	tp, ok := pendingTypes.types[tpKey]
	if !ok {
		return
	}
	defer delete(pendingTypes.types, tpKey)

	fmt.Printf("apply patching to %s\n", old.String())
	fns := scanMethods(old, tp)
	for _, p := range fns {
		patchFn(p.orig, p.patch)
	}
	fmt.Printf("patched methods: %v\n", len(fns))
}

func scanTypesFromPlugin(so string) {
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
	for c, tp := range newTps {
		validateTpVer(tp, ver)

		pendingTypes.Lock()
		k := convertToOrigTypeKey(c)
		pendingTypes.types[k] = tp
		pendingTypes.Unlock()
		fmt.Printf("patch detected: %s\n", k)
	}
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

var fileTimes = struct {
	sync.RWMutex
	times map[string]int64
}{times: map[string]int64{}}

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
	fileTimes.Lock()
	defer fileTimes.Unlock()

	items, _ := os.ReadDir(soDir)
	for _, item := range items {
		if strings.HasSuffix(item.Name(), ".so") {

			full := filepath.Join(soDir, item.Name())
			info, _ := item.Info()

			if t, ok := fileTimes.times[full]; !ok || info.ModTime().Unix() > t {
				fmt.Printf("detected new so: %s\n", item.Name())
				scanTypesFromPlugin(full)
				fileTimes.times[full] = info.ModTime().Unix()
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

func init() {
	startDaemon()
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
