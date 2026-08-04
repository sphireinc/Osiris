package osiris

import (
	"context"
	"fmt"
	"sync"
)

type GPUDevice struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Capabilities   []GPUCapability `json:"capabilities"`
	MaxElements    int             `json:"max_elements"`
	MaxBufferBytes int             `json:"max_buffer_bytes"`
	MaxRank        int             `json:"max_rank"`
}

type GPUDeviceRequest struct {
	ID                string          `json:"id,omitempty"`
	Required          []GPUCapability `json:"required,omitempty"`
	MaxElements       int             `json:"max_elements,omitempty"`
	MaxBufferBytes    int             `json:"max_buffer_bytes,omitempty"`
	MaxRank           int             `json:"max_rank,omitempty"`
	AllowCPUReference bool            `json:"allow_cpu_reference,omitempty"`
}

type GPUExecutionRequest struct {
	Kernel string           `json:"kernel"`
	Inputs []GPUValue       `json:"inputs"`
	Device GPUDeviceRequest `json:"device,omitempty"`
	Limits GPULimits        `json:"limits,omitempty"`
}

// GPUProvider owns device memory and event synchronization. Inputs are
// borrowed for Launch and must be copied by the provider before it returns;
// outputs are newly allocated and owned by the caller until Release.
type GPUProvider interface {
	Devices(context.Context) ([]GPUDevice, error)
	SelectDevice(context.Context, GPUDeviceRequest) (GPUDevice, error)
	Allocate(context.Context, GPUDevice, GPUValue) (*GPUBuffer, error)
	Launch(context.Context, GPUDevice, GPUKernel, []*GPUBuffer) (*GPUBuffer, *GPUEvent, error)
}

// GPUBuffer is an owned provider allocation. Read and Synchronize never expose
// the backing slice, and use after Release is rejected deterministically.
type GPUBuffer struct {
	mu       sync.RWMutex
	owner    string
	value    GPUValue
	event    *GPUEvent
	released bool
}

func newGPUBuffer(owner string, value GPUValue) *GPUBuffer {
	return &GPUBuffer{owner: owner, value: cloneGPUValue(value)}
}

func (b *GPUBuffer) setEvent(event *GPUEvent) {
	b.mu.Lock()
	b.event = event
	b.mu.Unlock()
}

func (b *GPUBuffer) replace(value GPUValue) {
	b.mu.Lock()
	b.value = cloneGPUValue(value)
	b.mu.Unlock()
}

func (b *GPUBuffer) Synchronize(ctx context.Context) error {
	if b == nil {
		return newGPUError(ErrGPUOwnership, "synchronize", "nil buffer", nil)
	}
	b.mu.RLock()
	released := b.released
	event := b.event
	b.mu.RUnlock()
	if released {
		return newGPUError(ErrGPUOwnership, "synchronize", "buffer was released", nil)
	}
	if event == nil {
		return nil
	}
	return event.Wait(ctx)
}

func (b *GPUBuffer) Read(ctx context.Context) (GPUValue, error) {
	if err := b.Synchronize(ctx); err != nil {
		return GPUValue{}, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.released {
		return GPUValue{}, newGPUError(ErrGPUOwnership, "read", "buffer was released", nil)
	}
	return cloneGPUValue(b.value), nil
}

func (b *GPUBuffer) Release() error {
	if b == nil {
		return newGPUError(ErrGPUOwnership, "release", "nil buffer", nil)
	}
	b.mu.Lock()
	if b.released {
		b.mu.Unlock()
		return nil
	}
	b.released = true
	b.value = GPUValue{}
	b.mu.Unlock()
	return nil
}

type GPUEvent struct {
	done  chan struct{}
	once  sync.Once
	errMu sync.RWMutex
	err   error
}

func newGPUEvent() *GPUEvent { return &GPUEvent{done: make(chan struct{})} }

func (e *GPUEvent) complete(err error) {
	e.once.Do(func() {
		e.errMu.Lock()
		e.err = err
		e.errMu.Unlock()
		close(e.done)
	})
}

func (e *GPUEvent) Wait(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-e.done:
		e.errMu.RLock()
		err := e.err
		e.errMu.RUnlock()
		return err
	case <-ctx.Done():
		return newGPUError(ErrGPUCancelled, "synchronize", "context cancelled while waiting for kernel", ctx.Err())
	}
}

func ExecuteGPU(ctx context.Context, artifact GPUArtifact, provider GPUProvider, request GPUExecutionRequest) (GPUValue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return GPUValue{}, newGPUError(ErrGPUCancelled, "execute", "context cancelled before launch", err)
	}
	if err := ValidateGPUArtifact(artifact); err != nil {
		return GPUValue{}, err
	}
	limits := stricterGPULimits(artifact.Limits, request.Limits)
	if provider == nil {
		if !request.Device.AllowCPUReference {
			return GPUValue{}, newGPUError(ErrGPUCapabilityUnavailable, "execute", "no provider supplied and CPU reference fallback is disabled", nil)
		}
		provider = NewDeterministicCPUProvider(limits)
	}
	device, err := provider.SelectDevice(ctx, request.Device)
	if err != nil {
		return GPUValue{}, err
	}
	if device.MaxElements > 0 && device.MaxElements < limits.MaxElements {
		limits.MaxElements = device.MaxElements
	}
	if device.MaxBufferBytes > 0 && device.MaxBufferBytes < limits.MaxBufferBytes {
		limits.MaxBufferBytes = device.MaxBufferBytes
	}
	if device.MaxRank > 0 && device.MaxRank < limits.MaxRank {
		limits.MaxRank = device.MaxRank
	}
	var kernel GPUKernel
	found := false
	for _, candidate := range artifact.Kernels {
		if candidate.Name == request.Kernel {
			kernel = candidate
			found = true
			break
		}
	}
	if !found {
		return GPUValue{}, newGPUError(ErrGPUArtifactMalformed, "execute", "kernel "+request.Kernel+" is not in artifact", nil)
	}
	if len(request.Inputs) != len(kernel.Inputs) {
		return GPUValue{}, newGPUError(ErrGPUArtifactMalformed, "execute", fmt.Sprintf("kernel %s expects %d inputs, got %d", kernel.Name, len(kernel.Inputs), len(request.Inputs)), nil)
	}
	inputs := make([]*GPUBuffer, len(request.Inputs))
	for i, value := range request.Inputs {
		if err := ValidateGPUValue(value, limits); err != nil {
			return GPUValue{}, err
		}
		inputs[i], err = provider.Allocate(ctx, device, value)
		if err != nil {
			for _, input := range inputs[:i] {
				_ = input.Release()
			}
			return GPUValue{}, err
		}
	}
	defer func() {
		for _, input := range inputs {
			_ = input.Release()
		}
	}()
	output, event, err := provider.Launch(ctx, device, kernel, inputs)
	if err != nil {
		return GPUValue{}, err
	}
	defer output.Release()
	if err := event.Wait(ctx); err != nil {
		return GPUValue{}, err
	}
	result, err := output.Read(ctx)
	if err != nil {
		return GPUValue{}, err
	}
	if err := ValidateGPUValue(result, limits); err != nil {
		return GPUValue{}, err
	}
	return result, nil
}

func cloneGPUValue(value GPUValue) GPUValue {
	return GPUValue{ABI: value.ABI, Type: value.Type, Shape: append([]int(nil), value.Shape...), Data: append([]float64(nil), value.Data...)}
}
