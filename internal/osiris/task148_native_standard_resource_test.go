package osiris

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestTask148NativeStandardStreamsAndRawHandlesExecute(t *testing.T) {
	got := runTask148NativeResourceProgram(t, `import "std/os" as os
func identity(value) => value
let raw = os.new_file(1, "raw-stdout")
let descriptor = os.fd(raw)
emit descriptor >= 0
emit os.with_handle(raw, identity) >= 0
let connection = os.syscall_conn(raw)
emit connection.read(identity) >= 0
emit os.name(raw)
emit os.close(raw)
emit os.fd(os.stdout) >= 0
emit os.name(os.stdin)
emit os.pipe()
`, "os.file_read", "os.file_write", "os.process_raw_handle", "os.file_metadata")
	if !strings.Contains(got, `"raw-stdout"`) || !strings.Contains(got, `"/dev/stdin"`) || !strings.Contains(got, "<host-resource>") {
		t.Fatalf("native standard/raw resource output = %q, want owned name and pipe resources", got)
	}
}

func TestTask148NativeSignalSubscriptionExecutes(t *testing.T) {
	signalNumber := int(syscall.SIGUSR1)
	got := runTask148NativeResourceProgram(t, fmt.Sprintf(`import "std/os" as os
let subscription = os.signal_subscribe("SIGUSR1")
let process = os.find_process(os.getpid())
emit os.process_signal(process, %d)
emit readchunk(subscription, 64, 1000)
emit os.close(subscription)
emit os.close(process)
`, signalNumber), "os.signal", "os.system_identity", "os.process_inspect", "os.process", "os.process_signal")
	if !strings.Contains(got, `"SIGUSR1"`) {
		t.Fatalf("native signal subscription output = %q, want SIGUSR1 event", got)
	}
}

func TestTask148NativeStandardResourceCapabilityGate(t *testing.T) {
	program, err := Parse("task148-native-standard-capability.osr", `import "std/os" as os
emit os.pipe()
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{}); err == nil || !strings.Contains(err.Error(), `native capability "os.file_read"`) {
		t.Fatalf("CompileC error = %v, want os.file_read capability diagnostic", err)
	}
}
