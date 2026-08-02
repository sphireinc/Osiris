package osiris

import (
	"context"
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

func mustBytes(value Value) []byte {
	data, err := bytesFromValue(value)
	if err != nil {
		panic(err)
	}
	return data
}
