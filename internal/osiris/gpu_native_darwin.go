//go:build darwin && cgo

package osiris

/*
#cgo darwin CFLAGS: -fobjc-arc
#cgo darwin LDFLAGS: -framework Foundation -framework Metal
#include <stdint.h>
#include <stddef.h>

uintptr_t osiris_metal_create_device(char *name, size_t name_cap);
void osiris_metal_release_device(uintptr_t device);
uintptr_t osiris_metal_allocate(uintptr_t device, const float *data, size_t count, char *err, size_t err_cap);
uintptr_t osiris_metal_launch(uintptr_t device, int op, uintptr_t a, size_t a_count, uintptr_t b, size_t b_count, size_t out_count, uintptr_t *out_buffer, char *err, size_t err_cap);
int osiris_metal_wait(uintptr_t event, char *err, size_t err_cap);
int osiris_metal_read(uintptr_t buffer, float *data, size_t count, char *err, size_t err_cap);
void osiris_metal_release_buffer(uintptr_t buffer);
void osiris_metal_release_event(uintptr_t event);
int osiris_metal_supports(int op);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	metalOpAdd = iota + 1
	metalOpSub
	metalOpMul
	metalOpDiv
	metalOpPow
	metalOpHypot
	metalOpAbs
	metalOpSqrt
	metalOpExp
	metalOpLog
	metalOpLog10
	metalOpLog2
	metalOpSin
	metalOpCos
	metalOpTan
	metalOpAsin
	metalOpAcos
	metalOpAtan
	metalOpFloor
	metalOpCeil
	metalOpRound
)

type metalGPUBackend struct {
	device C.uintptr_t
}

func newNativeGPUBackend(limits GPULimits) (nativeGPUBackend, GPUDevice, error) {
	var name [128]C.char
	device := C.osiris_metal_create_device(&name[0], C.size_t(len(name)))
	if device == 0 {
		return nil, GPUDevice{}, newGPUError(ErrGPUCapabilityUnavailable, "discover", "Metal did not expose a default device", nil)
	}
	backend := &metalGPUBackend{device: device}
	deviceName := C.GoString(&name[0])
	if deviceName == "" {
		deviceName = "Metal device"
	}
	return backend, GPUDevice{
		ID:             "metal-0",
		Name:           deviceName,
		Kind:           "metal",
		Capabilities:   []GPUCapability{GPUCapabilityNative, GPUCapabilityFloat32, GPUCapabilityTensor, GPUCapabilityMath},
		MaxElements:    limits.MaxElements,
		MaxBufferBytes: limits.MaxBufferBytes,
		MaxRank:        limits.MaxRank,
	}, nil
}

func (b *metalGPUBackend) close() error {
	if b == nil || b.device == 0 {
		return nil
	}
	C.osiris_metal_release_device(b.device)
	b.device = 0
	return nil
}

func (b *metalGPUBackend) allocate(data []float64) (uintptr, error) {
	values := make([]C.float, len(data))
	for i, value := range data {
		values[i] = C.float(value)
	}
	var errbuf [256]C.char
	handle := C.osiris_metal_allocate(b.device, (*C.float)(unsafe.Pointer(&values[0])), C.size_t(len(values)), &errbuf[0], C.size_t(len(errbuf)))
	if handle == 0 {
		return 0, fmt.Errorf("%s", C.GoString(&errbuf[0]))
	}
	return uintptr(handle), nil
}

func (b *metalGPUBackend) launch(op GPUKernelOp, inputs []nativeGPUInput, outputShape []int, outputCount int) (uintptr, uintptr, error) {
	metalOp, ok := metalOperation(op)
	if !ok || len(inputs) == 0 || len(inputs) > 2 {
		return 0, 0, fmt.Errorf("unsupported Metal operation %s", op)
	}
	var a, c uintptr
	var aCount, cCount int
	a, aCount = inputs[0].handle, inputs[0].count
	if len(inputs) == 2 {
		c, cCount = inputs[1].handle, inputs[1].count
	}
	var output C.uintptr_t
	var errbuf [256]C.char
	event := C.osiris_metal_launch(b.device, C.int(metalOp), C.uintptr_t(a), C.size_t(aCount), C.uintptr_t(c), C.size_t(cCount), C.size_t(outputCount), &output, &errbuf[0], C.size_t(len(errbuf)))
	if event == 0 || output == 0 {
		return 0, 0, fmt.Errorf("%s", C.GoString(&errbuf[0]))
	}
	return uintptr(output), uintptr(event), nil
}

func (b *metalGPUBackend) wait(event uintptr) error {
	var errbuf [256]C.char
	if C.osiris_metal_wait(C.uintptr_t(event), &errbuf[0], C.size_t(len(errbuf))) != 0 {
		return fmt.Errorf("%s", C.GoString(&errbuf[0]))
	}
	return nil
}

func (b *metalGPUBackend) read(buffer uintptr, shape []int, count int) (GPUValue, error) {
	values := make([]C.float, count)
	var errbuf [256]C.char
	if C.osiris_metal_read(C.uintptr_t(buffer), (*C.float)(unsafe.Pointer(&values[0])), C.size_t(count), &errbuf[0], C.size_t(len(errbuf))) != 0 {
		return GPUValue{}, fmt.Errorf("%s", C.GoString(&errbuf[0]))
	}
	data := make([]float64, count)
	for i, value := range values {
		data[i] = float64(value)
	}
	return NewGPUValue(data, shape)
}

func (b *metalGPUBackend) releaseBuffer(buffer uintptr) {
	if buffer != 0 {
		C.osiris_metal_release_buffer(C.uintptr_t(buffer))
	}
}

func (b *metalGPUBackend) releaseEvent(event uintptr) {
	if event != 0 {
		C.osiris_metal_release_event(C.uintptr_t(event))
	}
}

func (b *metalGPUBackend) supports(op GPUKernelOp) bool {
	metalOp, ok := metalOperation(op)
	return ok && C.osiris_metal_supports(C.int(metalOp)) != 0
}

func metalOperation(op GPUKernelOp) (int, bool) {
	operations := map[GPUKernelOp]int{
		GPUKernelAdd: metalOpAdd, GPUKernelSub: metalOpSub, GPUKernelMul: metalOpMul,
		GPUKernelDiv: metalOpDiv, GPUKernelPow: metalOpPow, GPUKernelHypot: metalOpHypot,
		GPUKernelAbs: metalOpAbs, GPUKernelSqrt: metalOpSqrt, GPUKernelExp: metalOpExp,
		GPUKernelLog: metalOpLog, GPUKernelLog10: metalOpLog10, GPUKernelLog2: metalOpLog2,
		GPUKernelSin: metalOpSin, GPUKernelCos: metalOpCos, GPUKernelTan: metalOpTan,
		GPUKernelAsin: metalOpAsin, GPUKernelAcos: metalOpAcos, GPUKernelAtan: metalOpAtan,
		GPUKernelFloor: metalOpFloor, GPUKernelCeil: metalOpCeil, GPUKernelRound: metalOpRound,
	}
	value, ok := operations[op]
	return value, ok
}
