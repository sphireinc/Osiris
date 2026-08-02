//go:build cgo && (darwin || linux || freebsd || netbsd || openbsd)

package osiris

/*
#include <stdint.h>
*/
import "C"

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
)

type ffiCallbackState struct {
	mu     sync.Mutex
	ctx    context.Context
	value  Value
	closed bool
}

var (
	ffiCallbackMu     sync.Mutex
	ffiCallbacks      = make(map[uintptr]*ffiCallbackState)
	ffiCallbackIDSeed atomic.Uintptr
)

func registerFFICallback(ctx context.Context, value Value) (uintptr, func(), error) {
	if value.kind != kindFunction || value.fn == nil {
		return 0, func() {}, fmt.Errorf("ffi callback must be a function")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id := ffiCallbackIDSeed.Add(1)
	state := &ffiCallbackState{ctx: ctx, value: value}
	ffiCallbackMu.Lock()
	ffiCallbacks[id] = state
	ffiCallbackMu.Unlock()
	return id, func() {
		state.mu.Lock()
		state.closed = true
		state.mu.Unlock()
		ffiCallbackMu.Lock()
		delete(ffiCallbacks, id)
		ffiCallbackMu.Unlock()
	}, nil
}

//export osiris_ffi_callback_dispatch
func osiris_ffi_callback_dispatch(id C.uintptr_t, argument C.int64_t) C.int64_t {
	ffiCallbackMu.Lock()
	state := ffiCallbacks[uintptr(id)]
	ffiCallbackMu.Unlock()
	if state == nil {
		return 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed || contextError(state.ctx) != nil {
		return 0
	}
	result, err := invokeFFICallback(state.ctx, state.value, int64(argument))
	if err != nil {
		return 0
	}
	return C.int64_t(result)
}

func invokeFFICallback(ctx context.Context, value Value, argument int64) (int64, error) {
	if value.kind != kindFunction || value.fn == nil {
		return 0, fmt.Errorf("ffi callback is not callable")
	}
	arg := intValue(bigIntForFFICallback(argument), 64)
	if value.fn.builtin != nil {
		result, err := value.fn.builtin([]Value{arg})
		if err != nil {
			return 0, err
		}
		return ffiCallbackIntegerResult(result)
	}
	if value.fn.contextBuiltin != nil {
		result, err := value.fn.contextBuiltin(ctx, []Value{arg})
		if err != nil {
			return 0, err
		}
		return ffiCallbackIntegerResult(result)
	}
	if value.fn.body == nil || value.fn.session == nil {
		return 0, fmt.Errorf("ffi callback has no executable body")
	}
	result, err := value.fn.session.ev.eval(&callExpr{
		callee: &literalExpr{value: value},
		args:   []Expr{&literalExpr{value: arg}},
	}, value.fn.session.scope)
	if err != nil {
		return 0, err
	}
	return ffiCallbackIntegerResult(result)
}

func ffiCallbackIntegerResult(value Value) (int64, error) {
	result, err := value.Int()
	if err != nil {
		return 0, fmt.Errorf("ffi callback must return an integer: %w", err)
	}
	return int64(result), nil
}

func bigIntForFFICallback(value int64) *big.Int { return big.NewInt(value) }
