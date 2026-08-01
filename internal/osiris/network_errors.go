package osiris

import (
	"context"
	"errors"
	"net"
)

// networkFailure keeps host networking details behind the structured error
// model. The raw host error is retained only as debug metadata by the common
// error constructor; public fields remain deterministic and redaction-safe.
func networkFailure(operation, resource string, host error) error {
	kind := ErrorKindNetwork
	message := operation + " failed"
	switch {
	case errors.Is(host, context.Canceled):
		kind = ErrorKindCancelled
		message = operation + " was cancelled"
	case errors.Is(host, context.DeadlineExceeded):
		kind = ErrorKindTimeout
		message = operation + " timed out"
	case host != nil:
		if _, ok := host.(*net.AddrError); ok {
			kind = ErrorKindInvalidArgument
		}
	}
	record := newOsirisError(kind, message).
		withMetadata("operation", stringValue(operation)).
		withMetadata("component", stringValue("network"))
	if resource != "" {
		record = record.withMetadata("resource", stringValue(resource))
	}
	if host != nil {
		record = withNativeErrorCode(record, host).
			withDebugMetadata("host_error", stringValue(host.Error()))
	}
	return record
}
