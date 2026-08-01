//go:build !cgo || (!darwin && !linux && !freebsd && !netbsd && !openbsd)

package osiris

import "fmt"

type ffiNativeLibrary struct{}

func openFFINativeLibrary(string) (ffiNativeLibrary, error) {
	return ffiNativeLibrary{}, fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) lookup(string) error {
	return fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) callInteger(string, []int64) (int64, error) {
	return 0, fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) callFloat(string, []float64) (float64, error) {
	return 0, fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) callString(string, []string) (string, error) {
	return "", fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) callConstBufferInteger(string, [][]byte) (int64, error) {
	return 0, fmt.Errorf("direct C FFI is unavailable on this platform")
}
func (ffiNativeLibrary) close() error { return nil }
