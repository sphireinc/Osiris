package osiris

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestStdOSTestRuntimeUsesAutomaticFakeProviders(t *testing.T) {
	rt := NewTestRuntime()
	rt.SetEnvironment(map[string]string{"TOKEN": "deterministic"})
	rt.SetCommandLineArgs([]string{"fixture"})

	if _, err := stdOSWriteFileContext(rt, context.Background(), []Value{stringValue("/todo.txt"), stringValue("buy milk"), intValue(bigIntFromInt64(0o644), 32)}); err != nil {
		t.Fatal(err)
	}
	data, err := stdOSReadFileContext(rt, context.Background(), []Value{stringValue("/todo.txt")})
	if err != nil || string(mustBytes(data)) != "buy milk" {
		t.Fatalf("read_file = %s, err = %v", data.String(), err)
	}
	if _, err := stdOSMkdirContext(rt, context.Background(), []Value{stringValue("/todos"), intValue(bigIntFromInt64(0o755), 32)}, false); err != nil {
		t.Fatal(err)
	}
	entries, err := stdOSReadDirContext(rt, context.Background(), []Value{stringValue("/")})
	if err != nil || entries.String() != `["todo.txt", "todos"]` {
		t.Fatalf("read_dir = %s, err = %v", entries.String(), err)
	}
	env, err := stdOSEnvValue(rt, context.Background(), []Value{stringValue("TOKEN")})
	if err != nil || env.String() != `some("deterministic")` {
		t.Fatalf("getenv = %s, err = %v", env.String(), err)
	}
	host, err := stdOSIdentity(rt, "hostname")
	if err != nil || host.String() != `"osiris-test"` {
		t.Fatalf("hostname = %s, err = %v", host.String(), err)
	}
	started, err := processSpawnContextValue(rt, context.Background(), []Value{stringValue("ignored"), listValue(nil), nullValue()})
	if err != nil {
		t.Fatal(err)
	}
	process, err := mapGetValue([]Value{started, stringValue("process")})
	if err != nil {
		t.Fatal(err)
	}
	process, err = unwrapOption(process)
	if err != nil {
		t.Fatal(err)
	}
	result, err := processWaitContextValue(rt, context.Background(), []Value{process})
	if err != nil || result.String() == "null" {
		t.Fatalf("fake process result = %s, err = %v", result.String(), err)
	}
}

func TestStdOSHostAccessRemainsCapabilityGated(t *testing.T) {
	rt := NewRuntime()
	if _, err := stdOSReadFileContext(rt, context.Background(), []Value{stringValue("does-not-exist")}); err == nil {
		t.Fatal("read_file succeeded without os.file_read")
	}
	rt.GrantCapability("os.file_read")
	if _, err := stdOSReadFileContext(rt, context.Background(), []Value{stringValue("does-not-exist")}); err == nil {
		t.Fatal("missing host file unexpectedly succeeded")
	}
}

func TestStdOSRootConfinementAndDerivedHandleInvalidation(t *testing.T) {
	rt := NewTestRuntime()
	if _, err := stdOSMkdirContext(rt, context.Background(), []Value{stringValue("/workspace"), intValue(bigIntFromInt64(0o755), 32)}, false); err != nil {
		t.Fatal(err)
	}
	root, err := stdOSOpenRoot(rt, []Value{stringValue("/workspace")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdOSRootWriteFileContext(rt, context.Background(), []Value{root, stringValue("todo.txt"), stringValue("buy milk"), intValue(bigIntFromInt64(0o644), 32)}); err != nil {
		t.Fatal(err)
	}
	data, err := stdOSRootReadFileContext(rt, context.Background(), []Value{root, stringValue("todo.txt")})
	if err != nil || string(mustBytes(data)) != "buy milk" {
		t.Fatalf("root read = %s, err = %v", data.String(), err)
	}
	if _, err := stdOSRootReadFileContext(rt, context.Background(), []Value{root, stringValue("../escape")}); err == nil || !strings.Contains(err.Error(), "confined") {
		t.Fatalf("root traversal error = %v", err)
	}
	handle, err := stdOSRootOpenContext(rt, context.Background(), []Value{root, stringValue("todo.txt")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdOSRootClose(rt, context.Background(), []Value{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := stdOSFileReadContext(rt, context.Background(), []Value{handle, intValue(bigIntFromInt64(1), 32)}); err == nil {
		t.Fatal("derived handle remained usable after root close")
	}
}

func TestStdOSFakeProvidersPreserveCapabilityChecksAndFlagValidation(t *testing.T) {
	rt := NewTestRuntime()
	rt.RevokeCapability("os.file_read")
	if _, err := stdOSReadFileContext(rt, context.Background(), []Value{stringValue("/missing")}); err == nil {
		t.Fatal("fake read succeeded after os.file_read was revoked")
	}
	if _, err := stdOSOpenFileContext(rt, context.Background(), []Value{stringValue("/write-only"), intValue(bigIntFromInt64(int64(os.O_WRONLY|os.O_CREATE)), 32), intValue(bigIntFromInt64(0o644), 32)}); err != nil {
		t.Fatalf("write-only fake open should not require read capability: %v", err)
	}
	if _, err := stdOSOpenFileContext(rt, context.Background(), []Value{stringValue("/bad-flags"), intValue(bigIntFromInt64(3), 32), intValue(bigIntFromInt64(0o644), 32)}); err == nil {
		t.Fatal("contradictory access flags were accepted")
	}
	missing := errorValueFromGoError(filesystemFailure(ErrorKindNotFound, "os.open", "/missing", "missing", nil))
	classified, err := stdOSErrorPredicate([]Value{missing}, ErrorKindNotFound)
	if err != nil || !classified.boo {
		t.Fatalf("not-found predicate = %s, err = %v", classified.String(), err)
	}
}

func TestStdOSApplicationCodeUsesTestProviderWithoutMocks(t *testing.T) {
	program, err := Parse("std-os-test.osr", `import "std/os" as os
let written = os.write_file("/todo.txt", "buy milk", 420)
emit [written, os.read_file("/todo.txt")]`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewTestRuntime()
	values, err := rt.Run(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || len(values[0].list) != 2 || !values[0].list[0].boo || string(mustBytes(values[0].list[1])) != "buy milk" {
		t.Fatalf("values = %v", values)
	}
}

func TestLanguageTestRunnerUsesDeterministicStdOSProviders(t *testing.T) {
	path := t.TempDir() + "/todo_test.osr"
	if err := os.WriteFile(path, []byte("import \"std/os\" as os\nlet ok = os.write_file(\"/todo.txt\", \"buy milk\", 420)\nemit [ok, os.read_file(\"/todo.txt\")]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"", "vm"} {
		results, err := RunLanguageTests(context.Background(), []string{path}, LanguageTestOptions{Backend: backend})
		if err != nil || len(results) != 1 || !results[0].Passed {
			t.Fatalf("backend %q results = %#v, err = %v", backend, results, err)
		}
	}
}

func mustBytes(value Value) []byte {
	data, err := bytesFromValue(value)
	if err != nil {
		panic(err)
	}
	return data
}
