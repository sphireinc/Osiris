package osiris

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestStableHostErrorKindMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"cancelled", context.Canceled, ErrorKindCancelled},
		{"deadline", context.DeadlineExceeded, ErrorKindTimeout},
		{"not found", os.ErrNotExist, ErrorKindNotFound},
		{"permission", os.ErrPermission, ErrorKindPermissionDenied},
		{"exists", os.ErrExist, ErrorKindAlreadyExists},
		{"closed", net.ErrClosed, ErrorKindIO},
		{"eof", io.EOF, ErrorKindOutOfRange},
		{"address", &net.AddrError{Err: "bad address", Addr: "["}, ErrorKindInvalidArgument},
		{"dial", &net.OpError{Op: "dial", Err: errors.New("refused")}, ErrorKindUnavailable},
		{"network operation", &net.OpError{Op: "read", Err: errors.New("reset")}, ErrorKindNetwork},
		{"would block", syscall.EAGAIN, ErrorKindUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyGoError(test.err); got != test.want {
				t.Fatalf("classifyGoError(%v) = %s, want %s", test.err, got, test.want)
			}
		})
	}
}

func TestNativeHostCodesAreDebugOnly(t *testing.T) {
	value := errorValueFromGoError(syscall.EAGAIN)
	record, err := value.AsError()
	if err != nil || record == nil {
		t.Fatalf("error record = %#v, %v", record, err)
	}
	if record.kind != ErrorKindUnavailable {
		t.Fatalf("would-block kind = %s", record.kind)
	}
	if got := record.debugMetadata["native_code"].String(); got != strconv.Quote(strconv.FormatInt(int64(syscall.EAGAIN), 10)) {
		t.Fatalf("native debug code = %s", got)
	}
	if strings.Contains(record.publicPairs().String(), "native_code") {
		t.Fatalf("native code escaped public metadata: %s", record.publicPairs())
	}
}

func TestNormalizeErrorRetainsSafePublicSource(t *testing.T) {
	err := normalizeError(context.Canceled, "<task-test>")
	var record *OsirisError
	if !errors.As(err, &record) || record.kind != ErrorKindCancelled || record.location == nil || record.location.Source != "<task-test>" {
		t.Fatalf("normalized record = %#v", record)
	}
}

func TestDeterministicSchedulerCancelsSiblingWorkAfterFailure(t *testing.T) {
	var ran []string
	scheduler := deterministicScheduler{debug: true}
	_, err := scheduler.run(context.Background(), []evalTask{
		{key: "later", run: func() (Value, error) { ran = append(ran, "later"); return nullValue(), nil }},
		{key: "failing", run: func() (Value, error) { ran = append(ran, "failing"); return nullValue(), errors.New("boom") }},
	})
	if err == nil || err.Error() != "task failing failed: boom" {
		t.Fatalf("scheduler error = %v", err)
	}
	if got, want := len(ran), 1; got != want || ran[0] != "failing" {
		t.Fatalf("ran = %v, want only failing task", ran)
	}
}

func TestRedactedErrorRemovesSecretsFromEveryObservableLayer(t *testing.T) {
	cause := newOsirisError(ErrorKindUnavailable, "token=child-leak").
		withMetadata("api_token", stringValue("metadata-leak")).
		withStack([]ErrorFrame{{Name: "password=stack-leak"}})
	err := newOsirisError(ErrorKindNetwork, "authorization: bearer-leak").
		withCause(cause).
		withMetadata("diagnostic", stringValue("secret=public-metadata-leak")).
		withDebugMetadata("token", stringValue("debug-leak")).
		withStack([]ErrorFrame{{Name: "secret=outer-stack-leak"}})

	redacted := err.Redacted()
	if redacted == nil || redacted.cause == nil {
		t.Fatalf("redacted error did not retain its cause chain: %#v", redacted)
	}
	if len(redacted.stack) != 0 || len(redacted.cause.stack) != 0 || len(redacted.debugMetadata) != 0 || len(redacted.cause.debugMetadata) != 0 {
		t.Fatalf("redacted error retained debug or stack details: %#v", redacted)
	}
	if _, found := redacted.cause.metadata["api_token"]; found {
		t.Fatalf("redacted error retained sensitive metadata: %#v", redacted.cause.metadata)
	}
	if got := redacted.metadata["diagnostic"].String(); got != `"secret=[REDACTED]"` {
		t.Fatalf("redacted public metadata = %s", got)
	}
	for _, leaked := range []string{"bearer-leak", "child-leak", "metadata-leak", "debug-leak", "stack-leak"} {
		if strings.Contains(redacted.String(), leaked) {
			t.Fatalf("redacted summary leaked %q: %s", leaked, redacted)
		}
	}
}
