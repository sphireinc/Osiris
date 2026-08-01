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
static int osiris_ffi_call_string(void* symbol, char** args, int count, char** result) {
	switch (count) {
	case 0: *result = (char*) ((const char* (*)(void))symbol)(); return *result ? 0 : 1;
	case 1: *result = (char*) ((const char* (*)(const char*))symbol)(args[0]); return *result ? 0 : 1;
	case 2: *result = (char*) ((const char* (*)(const char*,const char*))symbol)(args[0],args[1]); return *result ? 0 : 1;
	case 3: *result = (char*) ((const char* (*)(const char*,const char*,const char*))symbol)(args[0],args[1],args[2]); return *result ? 0 : 1;
	case 4: *result = (char*) ((const char* (*)(const char*,const char*,const char*,const char*))symbol)(args[0],args[1],args[2],args[3]); return *result ? 0 : 1;
	default: return 1;
	}
}
static int osiris_ffi_call_const_buffer_i64(void* symbol, const unsigned char* data0, uintptr_t length0, const unsigned char* data1, uintptr_t length1, int count, int64_t* result) {
	switch (count) {
	case 1: *result = ((int64_t (*)(const unsigned char*, uintptr_t))symbol)(data0, length0); return 0;
	case 2: *result = ((int64_t (*)(const unsigned char*, uintptr_t, const unsigned char*, uintptr_t))symbol)(data0, length0, data1, length1); return 0;
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

func (library ffiNativeLibrary) callString(name string, args []string) (string, error) {
	symbol, err := library.symbol(name)
	if err != nil {
		return "", err
	}
	cargs := make([]*C.char, len(args))
	for i, arg := range args {
		cargs[i] = C.CString(arg)
	}
	defer func() {
		for _, arg := range cargs {
			C.free(unsafe.Pointer(arg))
		}
	}()
	var first **C.char
	if len(cargs) > 0 {
		first = (**C.char)(unsafe.Pointer(&cargs[0]))
	}
	var result *C.char
	if C.osiris_ffi_call_string(symbol, first, C.int(len(cargs)), &result) != 0 || result == nil {
		return "", fmt.Errorf("foreign string call failed")
	}
	return C.GoString(result), nil
}

func (library ffiNativeLibrary) callConstBufferInteger(name string, args [][]byte) (int64, error) {
	symbol, err := library.symbol(name)
	if err != nil {
		return 0, err
	}
	data := make([]unsafe.Pointer, len(args))
	for i, arg := range args {
		if len(arg) > 0 {
			data[i] = C.CBytes(arg)
			defer C.free(data[i])
		}
	}
	var data0, data1 *C.uchar
	if len(data) > 0 {
		data0 = (*C.uchar)(data[0])
	}
	if len(data) > 1 {
		data1 = (*C.uchar)(data[1])
	}
	var result C.int64_t
	if C.osiris_ffi_call_const_buffer_i64(symbol, data0, C.uintptr_t(len(args[0])), data1, func() C.uintptr_t {
		if len(args) < 2 {
			return 0
		}
		return C.uintptr_t(len(args[1]))
	}(), C.int(len(args)), &result) != 0 {
		return 0, fmt.Errorf("unsupported const_buffer ABI arity")
	}
	return int64(result), nil
}

func (library ffiNativeLibrary) close() error {
	if library.handle != nil {
		C.dlclose(library.handle)
	}
	return nil
}
