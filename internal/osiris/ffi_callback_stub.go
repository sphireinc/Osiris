//go:build !cgo || (!darwin && !linux && !freebsd && !netbsd && !openbsd)

package osiris

import (
	"context"
	"fmt"
)

func registerFFICallback(context.Context, Value) (uintptr, func(), error) {
	return 0, func() {}, fmt.Errorf("direct C FFI callbacks are unavailable on this platform")
}
