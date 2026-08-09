package osiris

import (
	"context"
	"strings"
	"testing"
)

func TestNativeWindowsRuntimeComponentOwnsWindowsBoundaries(t *testing.T) {
	component := cRuntimeWindowsComponent
	for _, marker := range []string{
		"#if defined(_WIN32)",
		"CreateProcessW",
		"CREATE_SUSPENDED",
		"CREATE_UNICODE_ENVIRONMENT",
		"CreatePipe",
		"CreateJobObjectW",
		"AssignProcessToJobObject",
		"JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE",
		"WaitForSingleObject",
		"TerminateJobObject",
		"TerminateProcess",
		"GetEnvironmentStringsW",
		"osiris_win_environment_block",
		"osiris_win_environment_set",
		"CreateFileW",
		"ReadFile",
		"WriteFile",
		"SetFilePointerEx",
		"FlushFileBuffers",
		"HANDLE",
		"SOCKET",
		"WSAStartup",
		"WSAPoll",
		"closesocket",
		"GetFullPathNameW",
		"root path must be a bounded relative path",
		"ReadDirectoryChangesW",
		"OVERLAPPED",
		"CancelIoEx",
		"CloseHandle",
		"osiris_host_pipe",
		"osiris_host_standard_stream",
		"osiris_host_fd",
		"osiris_host_with_handle",
		"osiris_host_syscall_conn",
		"osiris_host_new_file",
		"osiris_host_signal_subscribe",
		"osiris_host_signal_read",
		"osiris_host_root_chown",
		"osiris_host_root_chtimes",
		"osiris_host_root_fs",
		"FSCTL_GET_REPARSE_POINT",
	} {
		if !strings.Contains(component, marker) {
			t.Fatalf("Windows runtime component missing %q", marker)
		}
	}
	for _, forbidden := range []string{"fork()", "execvp(", "openat(", "waitpid(", "poll(&pfd", "path_length", "Windows process environment overlay remains deferred", "Process groups and environment-overlay semantics remain explicit deferred boundaries"} {
		if strings.Contains(component, forbidden) {
			t.Fatalf("Windows runtime component contains POSIX implementation marker %q", forbidden)
		}
	}
	for _, symbol := range []string{
		"osiris_host_process_spawn", "osiris_host_process_wait", "osiris_host_process_kill",
		"osiris_host_file_open", "osiris_host_file_io", "osiris_host_root_open",
		"osiris_host_root_open_file", "osiris_host_root_read_file", "osiris_host_root_write_file",
		"osiris_host_watch", "osiris_host_watch_read", "onet_tcp_dial", "onet_tcp_listen",
		"onet_tcp_accept", "onet_read", "onet_write", "onet_udp_bind", "onet_udp_connect",
		"onet_udp_send", "onet_udp_receive", "onet_dns_lookup", "onet_close",
	} {
		if !strings.Contains(component, symbol) {
			t.Fatalf("Windows runtime component missing ABI symbol %s", symbol)
		}
	}
}

func TestNativeWindowsRuntimeComponentPrecedesPOSIXResourceComponents(t *testing.T) {
	program, err := Parse("windows-runtime-order.osr", "emit true\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	windows := strings.Index(artifact.Source, "OSIRIS_WINDOWS_COMPONENT")
	network := strings.Index(artifact.Source, "#if !defined(OSIRIS_WINDOWS_COMPONENT)")
	if windows < 0 || network < 0 || windows > network {
		t.Fatalf("Windows adapter is not emitted before native components: windows=%d network=%d", windows, network)
	}
}

func TestNativeWindowsRuntimeIsWiredBehindWindowsGuard(t *testing.T) {
	program, err := Parse("windows-runtime-hook.osr", "emit true\n")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(artifact.Source, "#if defined(_WIN32) && !defined(OSIRIS_FORCE_NONPOSIX)") || !strings.Contains(artifact.Source, "CreateProcessW") {
		t.Fatal("generated C is missing the guarded Windows HANDLE adapter")
	}
}
