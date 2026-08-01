//go:build cgo && (darwin || linux || freebsd || netbsd || openbsd)

package osiris

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

static char* osiris_ffi_error(const char* message) {
	if (!message) message = "foreign call failed";
	return strdup(message);
}
static void* osiris_ffi_open(const char* path, char** error) {
	dlerror();
	void* handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
	if (!handle) *error = osiris_ffi_error(dlerror());
	return handle;
}
static void* osiris_ffi_symbol(void* handle, const char* name, char** error) {
	dlerror();
	void* symbol = dlsym(handle, name);
	const char* detail = dlerror();
	if (detail) *error = osiris_ffi_error(detail);
	return symbol;
}
static int osiris_ffi_call_i64(void* symbol, int64_t* args, int count, int64_t* result) {
	switch (count) {
	case 0: *result = ((int64_t (*)(void))symbol)(); return 0;
	case 1: *result = ((int64_t (*)(int64_t))symbol)(args[0]); return 0;
	case 2: *result = ((int64_t (*)(int64_t,int64_t))symbol)(args[0],args[1]); return 0;
	case 3: *result = ((int64_t (*)(int64_t,int64_t,int64_t))symbol)(args[0],args[1],args[2]); return 0;
	case 4: *result = ((int64_t (*)(int64_t,int64_t,int64_t,int64_t))symbol)(args[0],args[1],args[2],args[3]); return 0;
	default: return 1;
	}
}
static int osiris_ffi_call_f64(void* symbol, double* args, int count, double* result) {
	switch (count) {
	case 0: *result = ((double (*)(void))symbol)(); return 0;
	case 1: *result = ((double (*)(double))symbol)(args[0]); return 0;
	case 2: *result = ((double (*)(double,double))symbol)(args[0],args[1]); return 0;
	case 3: *result = ((double (*)(double,double,double))symbol)(args[0],args[1],args[2]); return 0;
	case 4: *result = ((double (*)(double,double,double,double))symbol)(args[0],args[1],args[2],args[3]); return 0;
	default: return 1;
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type ffiNativeLibrary struct{ handle unsafe.Pointer }

func openFFINativeLibrary(path string) (ffiNativeLibrary, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var cerr *C.char
	handle := C.osiris_ffi_open(cpath, &cerr)
	if handle == nil {
		message := "unable to open foreign library"
		if cerr != nil {
			message = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}
		return ffiNativeLibrary{}, fmt.Errorf("%s", message)
	}
	return ffiNativeLibrary{handle: handle}, nil
}

func (library ffiNativeLibrary) lookup(name string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cerr *C.char
	if C.osiris_ffi_symbol(library.handle, cname, &cerr) == nil {
		message := "symbol not found"
		if cerr != nil {
			message = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func (library ffiNativeLibrary) symbol(name string) (unsafe.Pointer, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cerr *C.char
	symbol := C.osiris_ffi_symbol(library.handle, cname, &cerr)
	if symbol == nil {
		message := "symbol not found"
		if cerr != nil {
			message = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}
		return nil, fmt.Errorf("%s", message)
	}
	return symbol, nil
}

func (library ffiNativeLibrary) callInteger(name string, args []int64) (int64, error) {
	symbol, err := library.symbol(name)
	if err != nil {
		return 0, err
	}
	var result C.int64_t
	var first *C.int64_t
	if len(args) > 0 {
		first = (*C.int64_t)(unsafe.Pointer(&args[0]))
	}
	if C.osiris_ffi_call_i64(symbol, first, C.int(len(args)), &result) != 0 {
		return 0, fmt.Errorf("unsupported integer ABI arity")
	}
	return int64(result), nil
}

func (library ffiNativeLibrary) callFloat(name string, args []float64) (float64, error) {
	symbol, err := library.symbol(name)
	if err != nil {
		return 0, err
	}
	var result C.double
	var first *C.double
	if len(args) > 0 {
		first = (*C.double)(unsafe.Pointer(&args[0]))
	}
	if C.osiris_ffi_call_f64(symbol, first, C.int(len(args)), &result) != 0 {
		return 0, fmt.Errorf("unsupported floating ABI arity")
	}
	return float64(result), nil
}

func (library ffiNativeLibrary) close() error {
	if library.handle != nil {
		C.dlclose(library.handle)
	}
	return nil
}
