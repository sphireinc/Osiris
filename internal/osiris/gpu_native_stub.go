//go:build !darwin || !cgo

package osiris

type unavailableGPUBackend struct{}

func newNativeGPUBackend(GPULimits) (nativeGPUBackend, GPUDevice, error) {
	return nil, GPUDevice{}, newGPUError(ErrGPUCapabilityUnavailable, "discover", "no native GPU adapter is available on this host", nil)
}
