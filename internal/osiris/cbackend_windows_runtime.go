package osiris

// cRuntimeWindowsComponent is the Windows implementation of the native-C
// host-resource ABI. It is deliberately kept in a separate component: the
// POSIX adapter uses descriptor-relative syscalls and cannot be made correct
// by casting a HANDLE to an int. The component is selected by the generated C
// target's _WIN32 definition and supplies the same host and socket symbols.
//
// The implementation uses UTF-16 Win32 file/process APIs, HANDLE ownership,
// Winsock SOCKET ownership, CreateProcess pipes, confined rooted paths, and
// ReadDirectoryChangesW watches. Process groups and environment-overlay
// semantics remain explicit deferred boundaries owned by task 148B.
const cRuntimeWindowsComponent = `
#if defined(_WIN32) && !defined(OSIRIS_FORCE_NONPOSIX)
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <tlhelp32.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <signal.h>
#include <winioctl.h>

#define OSIRIS_WINDOWS_COMPONENT 1
#define OSIRIS_NATIVE_NETWORK_COMPONENT 1
#define OSIRIS_NATIVE_POSIX_NET 1
#define OSIRIS_HOST_RESOURCE_MAGIC UINT32_C(0x4f485253)
#define OSIRIS_NET_MAGIC UINT64_C(0x4f535249534e4554)
#define OSIRIS_HOST_MAX_IO (1024u * 1024u)
#define OSIRIS_NET_MAX_PACKET (1024u * 1024u)
#define OSIRIS_NET_MAX_ADDRESS 4096u
#define OSIRIS_NET_MAX_DNS 65536u
#define OSIRIS_HOST_PROCESS 1
#define OSIRIS_HOST_FILE 2
#define OSIRIS_HOST_ROOT 3
#define OSIRIS_HOST_STREAM 4
#define OSIRIS_HOST_WATCH 5
#define OSIRIS_HOST_SIGNAL 6
#define OSIRIS_NET_TCP 1
#define OSIRIS_NET_TCP_LISTENER 2
#define OSIRIS_NET_UDP 3

#ifndef POLLIN
#define POLLIN 0x0001
#endif
#ifndef POLLOUT
#define POLLOUT 0x0004
#endif
#ifndef ssize_t
typedef SSIZE_T ssize_t;
#endif
#define close(fd) closesocket((SOCKET)(fd))

typedef struct OHostResource OHostResource;
typedef struct {
  uint64_t magic;
  SOCKET fd;
  int kind;
  int closed;
} ONetHandle;

struct OHostResource {
  uint32_t magic;
  uint16_t kind;
  uint16_t closed;
  HANDLE fd;
  HANDLE input_fd;
  HANDLE output_fd;
  HANDLE error_fd;
  HANDLE process;
  HANDLE watch_event;
  DWORD pid;
  DWORD exit_code;
  int waited;
  int recursive;
  int64_t deadline_ms;
  wchar_t *path;
  unsigned char watch_buffer[65536];
  DWORD watch_bytes;
  DWORD watch_offset;
  char *watch_pending;
  size_t watch_pending_length;
  size_t watch_pending_offset;
  char *name;
  int signal_number;
  HANDLE signal_read;
  HANDLE signal_write;
  OHostResource *next;
};

static OHostResource *osiris_win_resources = NULL;
static int osiris_win_resources_atexit = 0;
static int osiris_win_wsa_ready = 0;
static HANDLE osiris_win_signal_write = NULL;
static int osiris_win_signal_number = 0;
static volatile sig_atomic_t osiris_win_signal_pending = 0;
static void (*osiris_win_previous_signal)(int) = SIG_DFL;
static void osiris_win_install_atexit(void);

static void osiris_win_signal_handler(int signal_number) {
  if (osiris_win_signal_write) osiris_win_signal_pending = (sig_atomic_t)signal_number;
}

static char *osiris_win_strdup(const char *value) {
  size_t length = value ? strlen(value) : 0;
  char *copy = (char *)malloc(length + 1);
  if (!copy) return NULL;
  if (length) memcpy(copy, value, length);
  copy[length] = 0;
  return copy;
}

static int64_t osiris_win_now_ms(void) { return (int64_t)GetTickCount64(); }

static OValue osiris_win_error(const char *operation, DWORD code) {
  char message[512];
  DWORD length = FormatMessageA(FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS, NULL, code, 0, message, sizeof(message), NULL);
  if (!length) snprintf(message, sizeof(message), "%s failed with Windows error %lu", operation, (unsigned long)code);
  while (length && (message[length - 1] == '\r' || message[length - 1] == '\n' || message[length - 1] == ' ')) message[--length] = 0;
  char detail[640]; snprintf(detail, sizeof(detail), "%s: %s", operation, message);
  return oerrorvalue("io", detail);
}

static OValue osiris_win_wsa_error(const char *operation, int code) {
  char message[256]; snprintf(message, sizeof(message), "%s failed with Winsock error %d", operation, code);
  return oerrorvalue("network", message);
}

static int osiris_win_utf8_to_wide(const char *data, size_t length, wchar_t **out) {
  if (!data || !out || length > INT_MAX || memchr(data, 0, length)) return 0;
  int needed = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, data, (int)length, NULL, 0);
  if (needed <= 0) return 0;
  wchar_t *copy = (wchar_t *)malloc(((size_t)needed + 1) * sizeof(wchar_t));
  if (!copy) return 0;
  if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, data, (int)length, copy, needed) != needed) { free(copy); return 0; }
  copy[needed] = 0; *out = copy; return 1;
}

static char *osiris_win_wide_to_utf8(const wchar_t *data, size_t length) {
  if (!data || length > INT_MAX) return NULL;
  int needed = WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, data, (int)length, NULL, 0, NULL, NULL);
  if (needed <= 0) return NULL;
  char *copy = (char *)malloc((size_t)needed + 1);
  if (!copy) return NULL;
  if (WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, data, (int)length, copy, needed, NULL, NULL) != needed) { free(copy); return NULL; }
  copy[needed] = 0; return copy;
}

static int osiris_win_string(OValue value, const char **data, size_t *length) {
  if (value.kind != O_STRING || !value.as.s.data || value.as.s.len > OSIRIS_HOST_MAX_IO || memchr(value.as.s.data, 0, value.as.s.len)) return 0;
  *data = value.as.s.data; *length = value.as.s.len; return 1;
}

static int osiris_win_i64(OValue value, int64_t *out) {
  if (value.kind == O_INT) { *out = value.as.i; return 1; }
  if (value.kind == O_UINT && value.as.u <= INT64_MAX) { *out = (int64_t)value.as.u; return 1; }
  return 0;
}

static int osiris_win_absolute_path(const wchar_t *input, wchar_t **output) {
  DWORD needed = GetFullPathNameW(input, 0, NULL, NULL);
  if (!needed || needed > OSIRIS_HOST_MAX_IO) return 0;
  wchar_t *copy = (wchar_t *)malloc(((size_t)needed + 1) * sizeof(wchar_t));
  if (!copy) return 0;
  if (!GetFullPathNameW(input, needed + 1, copy, NULL)) { free(copy); return 0; }
  *output = copy; return 1;
}

static int osiris_win_path_is_within(const wchar_t *root, const wchar_t *candidate) {
  size_t root_length = wcslen(root), candidate_length = wcslen(candidate);
  if (candidate_length < root_length || _wcsnicmp(root, candidate, root_length) != 0) return 0;
  return candidate_length == root_length || root[root_length - 1] == L'\\' || candidate[root_length] == L'\\';
}

static int osiris_win_relative_has_parent(const wchar_t *path) {
  const wchar_t *start = path; for (const wchar_t *cursor = path;; cursor++) { if (*cursor != L'\\' && *cursor != L'/' && *cursor != 0) continue; size_t length = (size_t)(cursor - start); if (length == 2 && start[0] == L'.' && start[1] == L'.') return 1; if (!*cursor) return 0; start = cursor + 1; }
}

static int osiris_win_path_has_reparse_point(const wchar_t *path, int allow_missing, int allow_final_reparse) {
  size_t length = wcslen(path), start = 0;
  for (size_t i = 0; i <= length; i++) {
    if (path[i] != L'\\' && path[i] != 0) continue;
    if (i < 3) continue;
    size_t part_length = i;
    wchar_t *part = (wchar_t *)malloc((part_length + 1) * sizeof(wchar_t));
    if (!part) return 1;
    memcpy(part, path, part_length * sizeof(wchar_t)); part[part_length] = 0;
    HANDLE handle = CreateFileW(part, FILE_READ_ATTRIBUTES, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT, NULL);
    DWORD error = GetLastError();
    if (handle == INVALID_HANDLE_VALUE) {
      free(part);
      if (allow_missing && i == length && (error == ERROR_FILE_NOT_FOUND || error == ERROR_PATH_NOT_FOUND)) return 0;
      return 1;
    }
    BY_HANDLE_FILE_INFORMATION info;
    int reparse = !GetFileInformationByHandle(handle, &info) || ((info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT) && !(allow_final_reparse && i == length));
    CloseHandle(handle); free(part); if (reparse) return 1; start = i + 1;
  }
  (void)start; return 0;
}

static int osiris_win_handle_is_within_root(OHostResource *root, HANDLE handle) {
  if (!root || !root->path || !handle || handle == INVALID_HANDLE_VALUE) return 0;
  DWORD needed = GetFinalPathNameByHandleW(handle, NULL, 0, FILE_NAME_NORMALIZED); if (!needed || needed > OSIRIS_HOST_MAX_IO) return 0;
  wchar_t *resolved = (wchar_t *)malloc(((size_t)needed + 1) * sizeof(wchar_t)); if (!resolved) return 0;
  DWORD written = GetFinalPathNameByHandleW(handle, resolved, needed + 1, FILE_NAME_NORMALIZED); const wchar_t *comparison = resolved; if (written >= 4 && !wmemcmp(resolved, L"\\\\?\\", 4)) comparison = resolved + 4; int within = written && osiris_win_path_is_within(root->path, comparison); free(resolved); return within;
}

static int osiris_win_root_path_with_options(OHostResource *root, OValue path_value, wchar_t **output, int allow_final_reparse) {
  const char *data = NULL; size_t length = 0;
  if (!root || root->closed || !osiris_win_string(path_value, &data, &length) || length == 0) return 0;
  wchar_t *relative = NULL;
  if (!osiris_win_utf8_to_wide(data, length, &relative)) return 0;
  if (relative[0] == L'\\' || relative[0] == L'/' || wcschr(relative, L':') || osiris_win_relative_has_parent(relative)) { free(relative); return 0; }
  wchar_t *joined = (wchar_t *)malloc((wcslen(root->path) + wcslen(relative) + 2) * sizeof(wchar_t));
  if (!joined) { free(relative); return 0; }
  wcscpy(joined, root->path); wcscat(joined, L"\\"); wcscat(joined, relative); free(relative);
  wchar_t *absolute = NULL;
  if (!osiris_win_absolute_path(joined, &absolute) || !osiris_win_path_is_within(root->path, absolute) || osiris_win_path_has_reparse_point(absolute, 1, allow_final_reparse)) { free(joined); free(absolute); return 0; }
  free(joined); *output = absolute; return 1;
}

static int osiris_win_root_path(OHostResource *root, OValue path_value, wchar_t **output) { return osiris_win_root_path_with_options(root, path_value, output, 0); }
static int osiris_win_root_inspection_path(OHostResource *root, OValue path_value, wchar_t **output) { return osiris_win_root_path_with_options(root, path_value, output, 1); }

static void osiris_win_close_handle(HANDLE *handle) {
  if (handle && *handle && *handle != INVALID_HANDLE_VALUE) { CloseHandle(*handle); *handle = NULL; }
}

static OHostResource *osiris_win_resource_new(int kind) {
  OHostResource *resource = (OHostResource *)calloc(1, sizeof(*resource));
  if (!resource) return NULL;
  resource->magic = OSIRIS_HOST_RESOURCE_MAGIC; resource->kind = (uint16_t)kind;
  resource->fd = resource->input_fd = resource->output_fd = resource->error_fd = NULL;
  resource->process = NULL; resource->watch_event = NULL; resource->signal_read = NULL; resource->signal_write = NULL; resource->deadline_ms = -1;
  resource->next = osiris_win_resources; osiris_win_resources = resource;
  osiris_win_install_atexit();
  return resource;
}

static OValue osiris_win_resource_value(OHostResource *resource) {
  if (!resource) return ofail("out of memory");
  return (OValue){O_RESOURCE, (uint16_t)resource->kind, {.resource = resource}};
}

static OHostResource *osiris_win_resource_get(OValue value, int kind) {
  if (value.kind != O_RESOURCE || !value.as.resource) { ofail("host resource handle expected"); return NULL; }
  OHostResource *resource = (OHostResource *)value.as.resource;
  if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->kind != kind || resource->closed) { ofail("host resource is closed or has the wrong kind"); return NULL; }
  return resource;
}

static void osiris_win_resource_close(OHostResource *resource) {
  if (!resource || resource->closed) return;
  if (resource->kind == OSIRIS_HOST_SIGNAL && osiris_win_signal_write == resource->signal_write) {
    osiris_win_signal_write = NULL;
    osiris_win_signal_number = 0;
    signal(resource->signal_number, osiris_win_previous_signal);
  }
  osiris_win_close_handle(&resource->fd); osiris_win_close_handle(&resource->input_fd); osiris_win_close_handle(&resource->output_fd); osiris_win_close_handle(&resource->error_fd); osiris_win_close_handle(&resource->process); osiris_win_close_handle(&resource->watch_event); osiris_win_close_handle(&resource->signal_read); osiris_win_close_handle(&resource->signal_write);
  free(resource->path); resource->path = NULL; free(resource->name); resource->name = NULL; free(resource->watch_pending); resource->watch_pending = NULL; resource->watch_pending_length = resource->watch_pending_offset = 0; resource->closed = 1;
}

static void osiris_win_resource_cleanup(void) { for (OHostResource *item = osiris_win_resources; item; item = item->next) osiris_win_resource_close(item); }

static OValue osiris_win_bytes(const unsigned char *data, size_t length) {
  if (length > OSIRIS_HOST_MAX_IO) return ofail("host I/O result exceeds the native bound");
  OValue *items = length ? (OValue *)malloc(length * sizeof(OValue)) : NULL;
  if (length && !items) return ofail("out of memory");
  for (size_t i = 0; i < length; i++) items[i] = ouintbits(data[i], 8);
  OValue result = olist(items, length); free(items); return result;
}

static int osiris_win_value_bytes(OValue value, unsigned char **out, size_t *length) {
  *out = NULL; *length = 0;
  if (value.kind == O_STRING) {
    if (value.as.s.len > OSIRIS_HOST_MAX_IO || memchr(value.as.s.data, 0, value.as.s.len)) return 0;
    *out = (unsigned char *)malloc(value.as.s.len ? value.as.s.len : 1); if (value.as.s.len && !*out) return 0;
    if (value.as.s.len) memcpy(*out, value.as.s.data, value.as.s.len); *length = value.as.s.len; return 1;
  }
  if (value.kind != O_LIST || value.as.list.len > OSIRIS_HOST_MAX_IO) return 0;
  *out = (unsigned char *)malloc(value.as.list.len ? value.as.list.len : 1); if (value.as.list.len && !*out) return 0;
  for (size_t i = 0; i < value.as.list.len; i++) { if (value.as.list.items[i].kind != O_UINT || value.as.list.items[i].as.u > 255) { free(*out); *out = NULL; return 0; } (*out)[i] = (unsigned char)value.as.list.items[i].as.u; }
  *length = value.as.list.len; return 1;
}

static OValue osiris_win_owned_string(const char *data, size_t length) {
  char *copy = (char *)malloc(length + 1); if (!copy) return ofail("out of memory"); memcpy(copy, data, length); copy[length] = 0;
  if (!otrack(copy)) { free(copy); return ofail("out of memory"); } return ostringview(copy, length);
}

static char *osiris_win_json_escape(const char *data, size_t length) {
  size_t capacity = length * 6 + 1; if (capacity < length) return NULL; char *result = (char *)malloc(capacity); if (!result) return NULL; size_t offset = 0;
  for (size_t i = 0; i < length; i++) { unsigned char value = (unsigned char)data[i]; const char *escape = NULL; switch (value) { case '\\': escape = "\\\\"; break; case '"': escape = "\\\""; break; case '\n': escape = "\\n"; break; case '\r': escape = "\\r"; break; case '\t': escape = "\\t"; break; default: break; } if (escape) { size_t size = strlen(escape); memcpy(result + offset, escape, size); offset += size; } else if (value < 0x20) { offset += (size_t)snprintf(result + offset, capacity - offset, "\\u%04x", value); } else result[offset++] = (char)value; }
  result[offset] = 0; return result;
}

static OValue osiris_host_process_stream(OValue value, int which);
static OHostResource *osiris_host_io_resource(OValue value);
static OValue osiris_host_process_value(OValue value) { OValue *member = omapget(value, "process"); return member ? *member : value; }
static OValue osiris_win_process_value(OValue value) { return osiris_host_process_value(value); }

static int osiris_win_command_line(wchar_t **output, OValue command, OValue arguments) {
  const char *command_data = NULL; size_t command_length = 0;
  if (!osiris_win_string(command, &command_data, &command_length) || !command_length || arguments.kind != O_LIST) return 0;
  wchar_t *command_w = NULL; if (!osiris_win_utf8_to_wide(command_data, command_length, &command_w)) return 0;
  size_t capacity = wcslen(command_w) + 3, length = 0; wchar_t *line = (wchar_t *)malloc(capacity * sizeof(wchar_t)); if (!line) { free(command_w); return 0; }
  line[length++] = L'"'; for (wchar_t *p = command_w; *p; p++) { if (*p == L'"') { if (length + 2 >= capacity) { capacity *= 2; line = (wchar_t *)realloc(line, capacity * sizeof(wchar_t)); if (!line) { free(command_w); return 0; } } line[length++] = L'\\'; } if (length + 2 >= capacity) { capacity *= 2; line = (wchar_t *)realloc(line, capacity * sizeof(wchar_t)); if (!line) { free(command_w); return 0; } } line[length++] = *p; } line[length++] = L'"'; free(command_w);
  for (size_t i = 0; i < arguments.as.list.len; i++) { const char *data = NULL; size_t bytes = 0; if (!osiris_win_string(arguments.as.list.items[i], &data, &bytes)) { free(line); return 0; } wchar_t *arg = NULL; if (!osiris_win_utf8_to_wide(data, bytes, &arg)) { free(line); return 0; } size_t extra = wcslen(arg) * 2 + 4; while (length + extra + 1 >= capacity) capacity *= 2; wchar_t *grown = (wchar_t *)realloc(line, capacity * sizeof(wchar_t)); if (!grown) { free(arg); free(line); return 0; } line = grown; line[length++] = L' '; line[length++] = L'"'; for (wchar_t *p = arg; *p; p++) { if (*p == L'"' || *p == L'\\') line[length++] = L'\\'; line[length++] = *p; } line[length++] = L'"'; free(arg); }
  line[length] = 0; *output = line; return 1;
}

static int osiris_win_process_options(OValue options, wchar_t **cwd) {
  *cwd = NULL; if (options.kind == O_NULL) return 1; if (options.kind != O_MAP) { ofail("process options must be a map or null"); return 0; }
  OValue *cwd_value = omapget(options, "cwd"); if (cwd_value) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(*cwd_value, &data, &length) || !length || !osiris_win_utf8_to_wide(data, length, cwd)) { ofail("process options cwd must be a valid non-empty UTF-8 path"); return 0; } }
  if (omapget(options, "env") || omapget(options, "inherit_env")) { free(*cwd); *cwd = NULL; ofail("Windows process environment overlay is deferred to task 148B"); return 0; }
  return 1;
}

static OValue osiris_host_process_spawn(OValue command, OValue arguments, OValue options) {
  wchar_t *line = NULL, *cwd = NULL; if (!osiris_win_command_line(&line, command, arguments) || !osiris_win_process_options(options, &cwd)) { free(line); free(cwd); return onull(); }
  SECURITY_ATTRIBUTES security = {sizeof(security), NULL, TRUE}; HANDLE child_in = NULL, parent_in = NULL, parent_out = NULL, child_out = NULL, parent_err = NULL, child_err = NULL;
  if (!CreatePipe(&child_in, &parent_in, &security, 0) || !CreatePipe(&parent_out, &child_out, &security, 0) || !CreatePipe(&parent_err, &child_err, &security, 0)) { DWORD error = GetLastError(); osiris_win_close_handle(&child_in); osiris_win_close_handle(&parent_in); osiris_win_close_handle(&parent_out); osiris_win_close_handle(&child_out); osiris_win_close_handle(&parent_err); osiris_win_close_handle(&child_err); free(line); free(cwd); return osiris_win_error("process_spawn.pipe", error); }
  SetHandleInformation(parent_in, HANDLE_FLAG_INHERIT, 0); SetHandleInformation(parent_out, HANDLE_FLAG_INHERIT, 0); SetHandleInformation(parent_err, HANDLE_FLAG_INHERIT, 0);
  STARTUPINFOW startup; PROCESS_INFORMATION process_info; memset(&startup, 0, sizeof(startup)); memset(&process_info, 0, sizeof(process_info)); startup.cb = sizeof(startup); startup.dwFlags = STARTF_USESTDHANDLES; startup.hStdInput = child_in; startup.hStdOutput = child_out; startup.hStdError = child_err;
  BOOL created = CreateProcessW(NULL, line, NULL, NULL, TRUE, CREATE_UNICODE_ENVIRONMENT, NULL, cwd, &startup, &process_info); DWORD error = created ? ERROR_SUCCESS : GetLastError(); osiris_win_close_handle(&child_in); osiris_win_close_handle(&child_out); osiris_win_close_handle(&child_err); free(line); free(cwd);
  if (!created) { osiris_win_close_handle(&parent_in); osiris_win_close_handle(&parent_out); osiris_win_close_handle(&parent_err); return osiris_win_error("process_spawn", error); }
  CloseHandle(process_info.hThread); OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_PROCESS); if (!resource) { TerminateProcess(process_info.hProcess, 1); CloseHandle(process_info.hProcess); osiris_win_close_handle(&parent_in); osiris_win_close_handle(&parent_out); osiris_win_close_handle(&parent_err); return ofail("out of memory"); }
  resource->process = process_info.hProcess; resource->pid = process_info.dwProcessId; resource->input_fd = parent_in; resource->output_fd = parent_out; resource->error_fd = parent_err;
  OValue process = osiris_win_resource_value(resource); OValue values[5] = {process, oint((int64_t)resource->pid), osiris_host_process_stream(process, 0), osiris_host_process_stream(process, 1), osiris_host_process_stream(process, 2)}; const char *keys[5] = {"process", "pid", "stdin", "stdout", "stderr"}; return omap(keys, values, 5);
}

static OValue osiris_host_process_wait(OValue value, OValue timeout) {
  OHostResource *resource = osiris_win_resource_get(osiris_win_process_value(value), OSIRIS_HOST_PROCESS); if (!resource) return onull(); int64_t milliseconds = -1; if (timeout.kind != O_NULL && (!osiris_win_i64(timeout, &milliseconds) || milliseconds < 0)) return ofail("process_wait timeout must be a non-negative integer or null"); DWORD wait_ms = milliseconds < 0 || milliseconds > 0xfffffffeLL ? INFINITE : (DWORD)milliseconds; DWORD result = WaitForSingleObject(resource->process, wait_ms); if (result == WAIT_TIMEOUT) { TerminateProcess(resource->process, 1); WaitForSingleObject(resource->process, INFINITE); return oerrorvalue("timeout", "process_wait deadline exceeded"); } if (result != WAIT_OBJECT_0 || !GetExitCodeProcess(resource->process, &resource->exit_code)) return osiris_win_error("process_wait", GetLastError()); resource->waited = 1; OValue values[3] = {oint((int64_t)resource->exit_code), obool(resource->exit_code == 0), ostring("")}; const char *keys[3] = {"status", "success", "signal"}; return omap(keys, values, 3);
}

static OValue osiris_host_process_find(OValue pid_value) { int64_t pid = 0; if (!osiris_win_i64(pid_value, &pid) || pid <= 0 || pid > UINT32_MAX) return ofail("find_process expects a positive process id"); HANDLE process = OpenProcess(SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION | PROCESS_TERMINATE, FALSE, (DWORD)pid); if (!process) return osiris_win_error("find_process", GetLastError()); OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_PROCESS); if (!resource) { CloseHandle(process); return ofail("out of memory"); } resource->process = process; resource->pid = (DWORD)pid; return osiris_win_resource_value(resource); }

static OValue osiris_host_process_state(OValue value) { OHostResource *resource = osiris_win_resource_get(osiris_win_process_value(value), OSIRIS_HOST_PROCESS); if (!resource) return onull(); if (!resource->waited) { if (WaitForSingleObject(resource->process, 0) != WAIT_OBJECT_0) return oerrorvalue("busy", "process_state is not available until the process exits"); if (!GetExitCodeProcess(resource->process, &resource->exit_code)) return osiris_win_error("process_state", GetLastError()); resource->waited = 1; } OValue values[3] = {oint((int64_t)resource->exit_code), obool(resource->exit_code == 0), ostring("")}; const char *keys[3] = {"status", "success", "signal"}; return omap(keys, values, 3); }

static OValue osiris_host_process_kill(OValue value, OValue signal_value) { OHostResource *resource = osiris_win_resource_get(osiris_win_process_value(value), OSIRIS_HOST_PROCESS); if (!resource) return onull(); if (signal_value.kind != O_NULL && !osiris_win_i64(signal_value, &(int64_t){0})) return ofail("process signal must be an integer"); if (!TerminateProcess(resource->process, 1) && GetLastError() != ERROR_PROCESS_NOT_FOUND) return osiris_win_error("process_signal", GetLastError()); return obool(1); }

static OValue osiris_host_process_release(OValue value) { OHostResource *resource = osiris_win_resource_get(osiris_win_process_value(value), OSIRIS_HOST_PROCESS); if (!resource) return onull(); if (!resource->waited && WaitForSingleObject(resource->process, 0) != WAIT_OBJECT_0) return oerrorvalue("busy", "process_release requires a waited process"); osiris_win_resource_close(resource); return obool(1); }

static OValue osiris_host_process_stream(OValue value, int which) { OHostResource *process = osiris_win_resource_get(osiris_win_process_value(value), OSIRIS_HOST_PROCESS); if (!process) return onull(); OHostResource *stream = osiris_win_resource_new(OSIRIS_HOST_STREAM); if (!stream) return ofail("out of memory"); stream->fd = which == 0 ? process->input_fd : (which == 1 ? process->output_fd : process->error_fd); if (!stream->fd) { osiris_win_resource_close(stream); return ofail("process stream is closed"); } if (which == 0) process->input_fd = NULL; else if (which == 1) process->output_fd = NULL; else process->error_fd = NULL; return osiris_win_resource_value(stream); }

static OValue osiris_host_pipe(void) {
  SECURITY_ATTRIBUTES security = {sizeof(security), NULL, TRUE}; HANDLE read_handle = NULL, write_handle = NULL;
  if (!CreatePipe(&read_handle, &write_handle, &security, 0)) return osiris_win_error("pipe", GetLastError());
  SetHandleInformation(read_handle, HANDLE_FLAG_INHERIT, 0); SetHandleInformation(write_handle, HANDLE_FLAG_INHERIT, 0);
  OHostResource *read_resource = osiris_win_resource_new(OSIRIS_HOST_STREAM), *write_resource = osiris_win_resource_new(OSIRIS_HOST_STREAM);
  if (!read_resource || !write_resource) { osiris_win_close_handle(&read_handle); osiris_win_close_handle(&write_handle); return ofail("out of memory"); }
  read_resource->fd = read_handle; write_resource->fd = write_handle;
  OValue values[2] = {osiris_win_resource_value(read_resource), osiris_win_resource_value(write_resource)};
  return olist(values, 2);
}

static OValue osiris_host_standard_stream(int which) {
  if (which < 0 || which > 2) return ofail("standard stream index must be 0, 1, or 2");
  DWORD standard = which == 0 ? STD_INPUT_HANDLE : (which == 1 ? STD_OUTPUT_HANDLE : STD_ERROR_HANDLE);
  HANDLE original = GetStdHandle(standard), duplicate = NULL;
  if (!original || original == INVALID_HANDLE_VALUE || !DuplicateHandle(GetCurrentProcess(), original, GetCurrentProcess(), &duplicate, 0, FALSE, DUPLICATE_SAME_ACCESS)) return osiris_win_error("standard_stream", GetLastError());
  OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_STREAM);
  if (!resource) { osiris_win_close_handle(&duplicate); return ofail("out of memory"); }
  resource->fd = duplicate; resource->name = osiris_win_strdup(which == 0 ? "CONIN$" : (which == 1 ? "CONOUT$" : "CONERR$"));
  return osiris_win_resource_value(resource);
}

static OValue osiris_host_fd(OValue value) {
  OHostResource *resource = osiris_host_io_resource(value);
  if (!resource) return onull();
  return ouintbits((uint64_t)(uintptr_t)resource->fd, 64);
}

static OValue osiris_host_name(OValue value) {
  OHostResource *resource = (value.kind == O_RESOURCE && value.as.resource) ? (OHostResource *)value.as.resource : NULL;
  if (!resource || resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) return ofail("name expects an open native resource");
  if (resource->name) return osiris_win_owned_string(resource->name, strlen(resource->name));
  if (!resource->path) return ostring("");
  char *name = osiris_win_wide_to_utf8(resource->path, wcslen(resource->path));
  if (!name) return ofail("resource name is not valid UTF-8");
  OValue result = osiris_win_owned_string(name, strlen(name)); free(name); return result;
}

static OValue osiris_host_new_file(OValue fd_value, OValue name_value) {
  uint64_t raw = 0; const char *name = NULL; size_t length = 0;
  if (fd_value.kind == O_UINT) raw = fd_value.as.u; else if (fd_value.kind == O_INT && fd_value.as.i >= 0) raw = (uint64_t)fd_value.as.i; else return ofail("new_file expects a raw HANDLE and UTF-8 name");
  if (!osiris_win_string(name_value, &name, &length)) return ofail("new_file expects a raw HANDLE and UTF-8 name");
  HANDLE source = (HANDLE)(uintptr_t)raw, duplicate = NULL;
  if (!source || source == INVALID_HANDLE_VALUE || !DuplicateHandle(GetCurrentProcess(), source, GetCurrentProcess(), &duplicate, 0, FALSE, DUPLICATE_SAME_ACCESS)) return osiris_win_error("new_file", GetLastError());
  OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_FILE);
  if (!resource) { osiris_win_close_handle(&duplicate); return ofail("out of memory"); }
  resource->fd = duplicate; resource->name = osiris_win_strdup(name); return osiris_win_resource_value(resource);
}

static int osiris_win_signal_number_for_name(OValue value) {
  const char *name = NULL; size_t length = 0;
  if (!osiris_win_string(value, &name, &length)) return 0;
  if ((length == 6 && !memcmp(name, "SIGINT", 6)) || (length == 3 && !memcmp(name, "INT", 3))) return SIGINT;
  if ((length == 7 && !memcmp(name, "SIGTERM", 7)) || (length == 4 && !memcmp(name, "TERM", 4))) return SIGTERM;
  if ((length == 8 && !memcmp(name, "SIGBREAK", 8)) || (length == 5 && !memcmp(name, "BREAK", 5))) return SIGBREAK;
  return 0;
}

static OValue osiris_host_signal_subscribe(OValue name_value) {
  int signal_number = osiris_win_signal_number_for_name(name_value); if (!signal_number) return ofail("signal_subscribe supports SIGINT, SIGTERM, and SIGBREAK on Windows");
  if (osiris_win_signal_write) return ofail("only one Windows signal subscription may be active");
  SECURITY_ATTRIBUTES security = {sizeof(security), NULL, TRUE}; HANDLE read_handle = NULL, write_handle = NULL;
  if (!CreatePipe(&read_handle, &write_handle, &security, 0)) return osiris_win_error("signal_subscribe", GetLastError());
  SetHandleInformation(read_handle, HANDLE_FLAG_INHERIT, 0); SetHandleInformation(write_handle, HANDLE_FLAG_INHERIT, 0);
  void (*previous)(int) = signal(signal_number, osiris_win_signal_handler);
  if (previous == SIG_ERR) { osiris_win_close_handle(&read_handle); osiris_win_close_handle(&write_handle); return osiris_win_error("signal_subscribe", GetLastError()); }
  OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_SIGNAL);
  if (!resource) { signal(signal_number, previous); osiris_win_close_handle(&read_handle); osiris_win_close_handle(&write_handle); return ofail("out of memory"); }
  resource->signal_number = signal_number; resource->signal_read = read_handle; resource->signal_write = write_handle; osiris_win_signal_write = write_handle; osiris_win_signal_number = signal_number; osiris_win_previous_signal = previous;
  return osiris_win_resource_value(resource);
}

static OValue osiris_host_signal_read(OValue value, OValue size_value, OValue timeout_value) {
  OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_SIGNAL); if (!resource) return onull(); int64_t requested = 0, timeout = -1;
  if (!osiris_win_i64(size_value, &requested) || requested < 0 || requested > OSIRIS_HOST_MAX_IO || (timeout_value.kind != O_NULL && (!osiris_win_i64(timeout_value, &timeout) || timeout < 0))) return ofail("signal read expects bounded size and timeout");
  int64_t start = osiris_win_now_ms(); while (!osiris_win_signal_pending) { if (timeout >= 0 && osiris_win_now_ms() - start >= timeout) return oerrorvalue("timeout", "signal read deadline exceeded"); Sleep(1); }
  unsigned char *bytes = (unsigned char *)malloc(requested ? (size_t)requested : 1); if (requested && !bytes) return ofail("out of memory"); size_t count = requested ? 1 : 0; if (count) bytes[0] = (unsigned char)osiris_win_signal_pending; osiris_win_signal_pending = 0; OValue result_value = osiris_win_bytes(bytes, count); free(bytes); return result_value;
}

static OValue osiris_host_file_open(OValue path, OValue flags, OValue mode) {
  const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("open expects a non-empty NUL-free path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("open expects valid UTF-8"); int64_t flag_value = 0; if (!osiris_win_i64(flags, &flag_value)) { free(wide); return ofail("open flags must be an integer"); }
  DWORD access = ((flag_value & 3) == 0) ? GENERIC_READ : (((flag_value & 3) == 2) ? (GENERIC_READ | GENERIC_WRITE) : GENERIC_WRITE); DWORD disposition = (flag_value & 512) ? ((flag_value & 64) ? CREATE_ALWAYS : TRUNCATE_EXISTING) : ((flag_value & 64) ? OPEN_ALWAYS : OPEN_EXISTING); HANDLE handle = CreateFileW(wide, access, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, disposition, FILE_ATTRIBUTE_NORMAL, NULL); DWORD error = GetLastError(); free(wide); if (handle == INVALID_HANDLE_VALUE) return osiris_win_error("open", error); OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_FILE); if (!resource) { CloseHandle(handle); return ofail("out of memory"); } resource->fd = handle; return osiris_win_resource_value(resource);
}

static OValue osiris_win_stat_path(const wchar_t *path, const char *name_override, int follow) {
  WIN32_FILE_ATTRIBUTE_DATA data; if (follow) { if (!GetFileAttributesExW(path, GetFileExInfoStandard, &data)) return osiris_win_error("stat", GetLastError()); } else { WIN32_FIND_DATAW found; HANDLE search = FindFirstFileW(path, &found); if (search == INVALID_HANDLE_VALUE) return osiris_win_error("lstat", GetLastError()); FindClose(search); data.dwFileAttributes = found.dwFileAttributes; data.ftCreationTime = found.ftCreationTime; data.ftLastAccessTime = found.ftLastAccessTime; data.ftLastWriteTime = found.ftLastWriteTime; data.nFileSizeHigh = found.nFileSizeHigh; data.nFileSizeLow = found.nFileSizeLow; }
  ULARGE_INTEGER size; size.HighPart = data.nFileSizeHigh; size.LowPart = data.nFileSizeLow; FILETIME local_time; SYSTEMTIME utc; FileTimeToLocalFileTime(&data.ftLastWriteTime, &local_time); FileTimeToSystemTime(&local_time, &utc); char modified[64]; snprintf(modified, sizeof(modified), "%04u-%02u-%02uT%02u:%02u:%02uZ", utc.wYear, utc.wMonth, utc.wDay, utc.wHour, utc.wMinute, utc.wSecond); const wchar_t *slash = wcsrchr(path, L'\\'); const wchar_t *base = slash ? slash + 1 : path; char *name = name_override ? osiris_win_strdup(name_override) : osiris_win_wide_to_utf8(base, wcslen(base)); if (!name) return ofail("out of memory"); OValue values[5] = {osiris_win_owned_string(name, strlen(name)), oint((int64_t)size.QuadPart), ouint((uint64_t)(data.dwFileAttributes | 0100000)), obool((data.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) != 0), ostring(modified)}; free(name); const char *keys[5] = {"name", "size", "mode", "dir", "mod_time"}; return omap(keys, values, 5);
}

static OValue osiris_host_stat(OValue path, int follow) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("stat expects a non-empty path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("stat expects valid UTF-8"); OValue result = osiris_win_stat_path(wide, NULL, follow); free(wide); return result; }

static OValue osiris_host_read_dir(OValue path) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("read_dir expects a non-empty path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("read_dir expects valid UTF-8"); size_t pattern_length = wcslen(wide) + 3; wchar_t *pattern = (wchar_t *)malloc(pattern_length * sizeof(wchar_t)); if (!pattern) { free(wide); return ofail("out of memory"); } wcscpy(pattern, wide); if (pattern[wcslen(pattern) - 1] != L'\\') wcscat(pattern, L"\\"); wcscat(pattern, L"*"); WIN32_FIND_DATAW item; HANDLE find = FindFirstFileW(pattern, &item); free(pattern); if (find == INVALID_HANDLE_VALUE) { DWORD error = GetLastError(); free(wide); return osiris_win_error("read_dir", error); } OValue *values = NULL; size_t count = 0, capacity = 0; do { if (!wcscmp(item.cFileName, L".") || !wcscmp(item.cFileName, L"..")) continue; char *name = osiris_win_wide_to_utf8(item.cFileName, wcslen(item.cFileName)); if (!name) { FindClose(find); free(values); free(wide); return ofail("read_dir path is not valid UTF-8"); } if (count == capacity) { capacity = capacity ? capacity * 2 : 16; OValue *grown = (OValue *)realloc(values, capacity * sizeof(OValue)); if (!grown) { free(name); FindClose(find); free(values); free(wide); return ofail("out of memory"); } values = grown; } values[count++] = osiris_win_owned_string(name, strlen(name)); free(name); } while (FindNextFileW(find, &item)); FindClose(find); free(wide); for (size_t i = 1; i < count; i++) { OValue item_value = values[i]; size_t j = i; while (j && strcmp(values[j - 1].as.s.data, item_value.as.s.data) > 0) { values[j] = values[j - 1]; j--; } values[j] = item_value; } OValue result = olist(values, count); free(values); return result; }

static int osiris_win_make_dirs(const wchar_t *path) { wchar_t *copy = NULL; if (!osiris_win_absolute_path(path, &copy)) return 0; size_t length = wcslen(copy); for (size_t i = 3; i < length; i++) if (copy[i] == L'\\') { copy[i] = 0; if (!CreateDirectoryW(copy, NULL) && GetLastError() != ERROR_ALREADY_EXISTS) { free(copy); return 0; } copy[i] = L'\\'; } BOOL result = CreateDirectoryW(copy, NULL); if (!result && GetLastError() == ERROR_ALREADY_EXISTS) { DWORD attrs = GetFileAttributesW(copy); result = attrs != INVALID_FILE_ATTRIBUTES && (attrs & FILE_ATTRIBUTE_DIRECTORY); } free(copy); return result != 0; }

static int osiris_win_remove_tree(const wchar_t *path) { DWORD attrs = GetFileAttributesW(path); if (attrs == INVALID_FILE_ATTRIBUTES) return GetLastError() == ERROR_FILE_NOT_FOUND; if (attrs & FILE_ATTRIBUTE_REPARSE_POINT) return DeleteFileW(path) || RemoveDirectoryW(path); if (!(attrs & FILE_ATTRIBUTE_DIRECTORY)) return DeleteFileW(path) != 0; size_t length = wcslen(path) + 3; wchar_t *pattern = (wchar_t *)malloc(length * sizeof(wchar_t)); if (!pattern) return 0; swprintf(pattern, length, L"%s\\*", path); WIN32_FIND_DATAW item; HANDLE find = FindFirstFileW(pattern, &item); free(pattern); if (find != INVALID_HANDLE_VALUE) { do { if (!wcscmp(item.cFileName, L".") || !wcscmp(item.cFileName, L"..")) continue; size_t child_length = wcslen(path) + wcslen(item.cFileName) + 2; wchar_t *child = (wchar_t *)malloc(child_length * sizeof(wchar_t)); if (!child) { FindClose(find); return 0; } swprintf(child, child_length, L"%s\\%s", path, item.cFileName); int ok = osiris_win_remove_tree(child); free(child); if (!ok) { FindClose(find); return 0; } } while (FindNextFileW(find, &item)); FindClose(find); } return RemoveDirectoryW(path) != 0; }

static OValue osiris_host_mkdir(OValue path, OValue mode, int all) { (void)mode; const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("mkdir expects a non-empty path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("mkdir expects valid UTF-8"); int ok = all ? osiris_win_make_dirs(wide) : (CreateDirectoryW(wide, NULL) != 0); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error(all ? "mkdir_all" : "mkdir", error); }
static OValue osiris_host_remove(OValue path, int all) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("remove expects a non-empty path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("remove expects valid UTF-8"); int ok = all ? osiris_win_remove_tree(wide) : (DeleteFileW(wide) || RemoveDirectoryW(wide)); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error(all ? "remove_all" : "remove", error); }
static OValue osiris_host_truncate(OValue path, OValue size) { const char *data = NULL; size_t length = 0; int64_t requested = 0; if (!osiris_win_string(path, &data, &length) || !osiris_win_i64(size, &requested) || requested < 0) return ofail("truncate expects a path and non-negative size"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("truncate expects valid UTF-8"); HANDLE handle = CreateFileW(wide, GENERIC_WRITE, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL); free(wide); if (handle == INVALID_HANDLE_VALUE) return osiris_win_error("truncate", GetLastError()); LARGE_INTEGER position; position.QuadPart = requested; BOOL ok = SetFilePointerEx(handle, position, NULL, FILE_BEGIN) && SetEndOfFile(handle); DWORD error = GetLastError(); CloseHandle(handle); return ok ? obool(1) : osiris_win_error("truncate", error); }
static OValue osiris_host_rename(OValue old_path, OValue new_path) { const char *old_data = NULL, *new_data = NULL; size_t old_length = 0, new_length = 0; if (!osiris_win_string(old_path, &old_data, &old_length) || !osiris_win_string(new_path, &new_data, &new_length)) return ofail("rename expects two paths"); wchar_t *old_w = NULL, *new_w = NULL; if (!osiris_win_utf8_to_wide(old_data, old_length, &old_w) || !osiris_win_utf8_to_wide(new_data, new_length, &new_w)) { free(old_w); free(new_w); return ofail("rename expects valid UTF-8"); } BOOL ok = MoveFileExW(old_w, new_w, MOVEFILE_REPLACE_EXISTING); DWORD error = GetLastError(); free(old_w); free(new_w); return ok ? obool(1) : osiris_win_error("rename", error); }
static OValue osiris_host_chmod(OValue path, OValue mode) { (void)mode; const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length)) return ofail("chmod expects a path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("chmod expects valid UTF-8"); DWORD attrs = GetFileAttributesW(wide); if (attrs == INVALID_FILE_ATTRIBUTES) { DWORD error = GetLastError(); free(wide); return osiris_win_error("chmod", error); } BOOL ok = SetFileAttributesW(wide, attrs & ~FILE_ATTRIBUTE_READONLY); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("chmod", error); }
static OValue osiris_host_link(OValue old_path, OValue new_path, int symbolic) { const char *old_data = NULL, *new_data = NULL; size_t old_length = 0, new_length = 0; if (!osiris_win_string(old_path, &old_data, &old_length) || !osiris_win_string(new_path, &new_data, &new_length)) return ofail("link expects two paths"); wchar_t *old_w = NULL, *new_w = NULL; if (!osiris_win_utf8_to_wide(old_data, old_length, &old_w) || !osiris_win_utf8_to_wide(new_data, new_length, &new_w)) { free(old_w); free(new_w); return ofail("link expects valid UTF-8"); } BOOL ok = symbolic ? CreateSymbolicLinkW(new_w, old_w, 0) : CreateHardLinkW(new_w, old_w, NULL); DWORD error = GetLastError(); free(old_w); free(new_w); return ok ? obool(1) : osiris_win_error(symbolic ? "symlink" : "link", error); }
static OValue osiris_win_readlink_path(const wchar_t *path) {
  HANDLE handle = CreateFileW(path, 0, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_FLAG_OPEN_REPARSE_POINT | FILE_FLAG_BACKUP_SEMANTICS, NULL);
  if (handle == INVALID_HANDLE_VALUE) return osiris_win_error("readlink", GetLastError());
  unsigned char buffer[MAXIMUM_REPARSE_DATA_BUFFER_SIZE]; DWORD returned = 0;
  if (!DeviceIoControl(handle, FSCTL_GET_REPARSE_POINT, NULL, 0, buffer, sizeof(buffer), &returned, NULL)) { DWORD error = GetLastError(); CloseHandle(handle); return osiris_win_error("readlink", error); }
  CloseHandle(handle); REPARSE_DATA_BUFFER *data = (REPARSE_DATA_BUFFER *)buffer;
  if (data->ReparseTag != IO_REPARSE_TAG_SYMLINK) return ofail("readlink target is not a symbolic link");
  WCHAR *target = data->SymbolicLinkReparseBuffer.PathBuffer + data->SymbolicLinkReparseBuffer.PrintNameOffset / sizeof(WCHAR);
  size_t target_length = data->SymbolicLinkReparseBuffer.PrintNameLength / sizeof(WCHAR); const wchar_t prefix[] = L"\\??\\";
  if (target_length >= 4 && !wmemcmp(target, prefix, 4)) { target += 4; target_length -= 4; }
  char *utf8 = osiris_win_wide_to_utf8(target, target_length); if (!utf8) return ofail("readlink target is not valid UTF-8"); OValue result = osiris_win_owned_string(utf8, strlen(utf8)); free(utf8); return result;
}

static OValue osiris_host_readlink(OValue path) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("readlink expects a non-empty path"); wchar_t *wide = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide)) return ofail("readlink expects valid UTF-8"); OValue result = osiris_win_readlink_path(wide); free(wide); return result; }

static OHostResource *osiris_host_io_resource(OValue value) { if (value.kind != O_RESOURCE || !value.as.resource) { ofail("host stream or file handle expected"); return NULL; } OHostResource *resource = (OHostResource *)value.as.resource; if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) { ofail("host stream or file handle is closed or invalid"); return NULL; } return resource; }
static OValue osiris_host_file_close(OValue value) { OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_FILE); if (!resource) return onull(); osiris_win_resource_close(resource); return obool(1); }
static OValue osiris_host_file_io(OValue value, OValue data_or_size, OValue offset, OValue timeout, int writing, int positioned) { (void)timeout; OHostResource *resource = osiris_host_io_resource(value); if (!resource) return onull(); unsigned char *bytes = NULL; size_t length = 0; if (writing) { if (!osiris_win_value_bytes(data_or_size, &bytes, &length)) return ofail("write expects bounded byte data"); } else { int64_t requested = 0; if (!osiris_win_i64(data_or_size, &requested) || requested < 0 || requested > OSIRIS_HOST_MAX_IO) return ofail("read expects a bounded size"); length = (size_t)requested; bytes = (unsigned char *)malloc(length ? length : 1); if (length && !bytes) return ofail("out of memory"); }
  LARGE_INTEGER saved_position, requested_position; int restored = 0; if (positioned) { int64_t position = 0; if (!osiris_win_i64(offset, &position) || position < 0) { free(bytes); return ofail("positioned I/O expects a non-negative offset"); } requested_position.QuadPart = 0; if (!SetFilePointerEx(resource->fd, requested_position, &saved_position, FILE_CURRENT)) { DWORD error = GetLastError(); free(bytes); return osiris_win_error("positioned_io", error); } requested_position.QuadPart = position; if (!SetFilePointerEx(resource->fd, requested_position, NULL, FILE_BEGIN)) { DWORD error = GetLastError(); free(bytes); return osiris_win_error("positioned_io", error); } restored = 1; }
  DWORD transferred = 0; BOOL ok = writing ? WriteFile(resource->fd, bytes, (DWORD)length, &transferred, NULL) : ReadFile(resource->fd, bytes, (DWORD)length, &transferred, NULL); DWORD error = GetLastError(); if (restored) { LARGE_INTEGER restore; restore.QuadPart = saved_position.QuadPart; if (!SetFilePointerEx(resource->fd, restore, NULL, FILE_BEGIN) && ok) { ok = FALSE; error = GetLastError(); } } if (!ok) { free(bytes); return osiris_win_error(writing ? "write" : "read", error); } if (writing) { free(bytes); return ouintbits(transferred, 64); } OValue result = osiris_win_bytes(bytes, transferred); free(bytes); return result; }
static OValue osiris_host_file_write_string(OValue value, OValue string_value, OValue timeout) { return osiris_host_file_io(value, string_value, onull(), timeout, 1, 0); }
static OValue osiris_host_file_seek(OValue value, OValue offset, OValue whence) { OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_FILE); if (!resource) return onull(); int64_t position = 0, origin = 0; if (!osiris_win_i64(offset, &position) || !osiris_win_i64(whence, &origin) || origin < 0 || origin > 2) return ofail("seek_file expects a valid offset and origin"); LARGE_INTEGER move; move.QuadPart = position; LARGE_INTEGER result; if (!SetFilePointerEx(resource->fd, move, &result, (DWORD)origin)) return osiris_win_error("seek_file", GetLastError()); return oint(result.QuadPart); }
static OValue osiris_host_file_sync(OValue value) { OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_FILE); if (!resource) return onull(); return FlushFileBuffers(resource->fd) ? onull() : osiris_win_error("sync", GetLastError()); }
static OValue osiris_host_resource_deadline(OValue value, OValue timeout) { OHostResource *resource = (value.kind == O_RESOURCE) ? (OHostResource *)value.as.resource : NULL; if (!resource || resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) return ofail("host resource is closed"); int64_t milliseconds = -1; if (timeout.kind != O_NULL && (!osiris_win_i64(timeout, &milliseconds) || milliseconds < 0)) return ofail("deadline expects a non-negative integer or null"); resource->deadline_ms = milliseconds < 0 ? -1 : osiris_win_now_ms() + milliseconds; return onull(); }

static OValue osiris_host_root_open(OValue path) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path, &data, &length) || !length) return ofail("open_root expects a non-empty path"); wchar_t *wide = NULL, *absolute = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide) || !osiris_win_absolute_path(wide, &absolute)) { free(wide); free(absolute); return ofail("open_root expects a valid path"); } HANDLE handle = CreateFileW(absolute, GENERIC_READ | FILE_LIST_DIRECTORY, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_FLAG_BACKUP_SEMANTICS, NULL); DWORD error = GetLastError(); if (handle == INVALID_HANDLE_VALUE) { free(wide); free(absolute); return osiris_win_error("open_root", error); } DWORD final_length = GetFinalPathNameByHandleW(handle, NULL, 0, FILE_NAME_NORMALIZED); wchar_t *final_path = final_length ? (wchar_t *)malloc(((size_t)final_length + 1) * sizeof(wchar_t)) : NULL; if (!final_path || !GetFinalPathNameByHandleW(handle, final_path, final_length + 1, FILE_NAME_NORMALIZED)) { free(final_path); CloseHandle(handle); free(wide); free(absolute); return ofail("open_root could not resolve its final HANDLE path"); } if (final_length >= 4 && !wmemcmp(final_path, L"\\\\?\\", 4)) memmove(final_path, final_path + 4, (size_t)(final_length - 3) * sizeof(wchar_t)); OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_ROOT); if (!resource) { CloseHandle(handle); free(wide); free(absolute); free(final_path); return ofail("out of memory"); } free(wide); free(absolute); resource->fd = handle; resource->path = final_path; return osiris_win_resource_value(resource); }
static HANDLE osiris_win_root_open_handle(OValue root_value, OValue path_value, DWORD access, DWORD disposition, DWORD flags, const char *operation) { OHostResource *root = osiris_win_resource_get(root_value, OSIRIS_HOST_ROOT); wchar_t *path = NULL; if (!root || !osiris_win_root_path(root, path_value, &path)) { ofail("root path must be a bounded relative path without .."); return NULL; } HANDLE handle = CreateFileW(path, access, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, disposition, flags, NULL); DWORD error = GetLastError(); free(path); if (handle == INVALID_HANDLE_VALUE) { osiris_win_error(operation, error); return NULL; } if (!osiris_win_handle_is_within_root(root, handle)) { CloseHandle(handle); ofail("root operation resolved outside its root"); return NULL; } return handle; }
static OValue osiris_host_root_open_file(OValue root, OValue path, OValue flags, OValue mode) { (void)mode; int64_t flag_value = 0; if (!osiris_win_i64(flags, &flag_value)) return ofail("root_open flags must be an integer"); DWORD access = ((flag_value & 3) == 0) ? GENERIC_READ : (((flag_value & 3) == 2) ? (GENERIC_READ | GENERIC_WRITE) : GENERIC_WRITE); DWORD disposition = (flag_value & 512) ? ((flag_value & 64) ? CREATE_ALWAYS : TRUNCATE_EXISTING) : ((flag_value & 64) ? OPEN_ALWAYS : OPEN_EXISTING); HANDLE handle = osiris_win_root_open_handle(root, path, access, disposition, FILE_ATTRIBUTE_NORMAL, "root_open"); if (!handle) return onull(); OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_FILE); if (!resource) { CloseHandle(handle); return ofail("out of memory"); } resource->fd = handle; return osiris_win_resource_value(resource); }
static OValue osiris_host_root_create(OValue root, OValue path) { return osiris_host_root_open_file(root, path, oint(514), oint(0666)); }
static OValue osiris_host_root_read_file(OValue root, OValue path) { OValue file = osiris_host_root_open_file(root, path, oint(0), oint(0)); if (file.kind != O_RESOURCE) return file; OValue result = osiris_host_file_io(file, ouintbits(OSIRIS_HOST_MAX_IO, 64), onull(), onull(), 0, 0); osiris_host_file_close(file); return result; }
static OValue osiris_host_root_write_file(OValue root, OValue path, OValue data, OValue mode) { OValue file = osiris_host_root_open_file(root, path, oint(514), mode); if (file.kind != O_RESOURCE) return file; OValue result = osiris_host_file_io(file, data, onull(), onull(), 1, 0); osiris_host_file_close(file); return result.kind == O_UINT ? obool(1) : result; }
static OValue osiris_host_root_stat(OValue root, OValue path, int follow) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !(follow ? osiris_win_root_path(resource, path, &wide) : osiris_win_root_inspection_path(resource, path, &wide))) return ofail("root path must be relative and confined"); OValue result = osiris_win_stat_path(wide, NULL, follow); free(wide); return result; }
static OValue osiris_host_root_readlink(OValue root, OValue path) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_inspection_path(resource, path, &wide)) return ofail("root_readlink path must be relative and confined"); OValue result = osiris_win_readlink_path(wide); free(wide); return result; }
static OValue osiris_host_root_link(OValue root, OValue old_path, OValue new_path, int symbolic) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *old_w = NULL, *new_w = NULL; if (!resource || !osiris_win_root_path(resource, old_path, &old_w) || !osiris_win_root_path(resource, new_path, &new_w)) { free(old_w); free(new_w); return ofail("root link paths must be relative and confined"); } BOOL ok = symbolic ? CreateSymbolicLinkW(new_w, old_w, 0) : CreateHardLinkW(new_w, old_w, NULL); DWORD error = GetLastError(); free(old_w); free(new_w); return ok ? obool(1) : osiris_win_error(symbolic ? "root_symlink" : "root_link", error); }
static OValue osiris_host_root_mkdir(OValue root, OValue path, OValue mode) { (void)mode; OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_path(resource, path, &wide)) return ofail("root_mkdir path must be relative and confined"); BOOL ok = CreateDirectoryW(wide, NULL); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("root_mkdir", error); }
static OValue osiris_host_root_mkdir_all(OValue root, OValue path, OValue mode) { (void)mode; OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_path(resource, path, &wide)) return ofail("root_mkdir_all path must be relative and confined"); int ok = osiris_win_make_dirs(wide); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("root_mkdir_all", error); }
static OValue osiris_host_root_remove(OValue root, OValue path) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_path(resource, path, &wide)) return ofail("root_remove path must be relative and confined"); BOOL ok = DeleteFileW(wide) || RemoveDirectoryW(wide); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("root_remove", error); }
static OValue osiris_host_root_remove_all(OValue root, OValue path) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_path(resource, path, &wide)) return ofail("root_remove_all path must be relative and confined"); int ok = osiris_win_remove_tree(wide); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("root_remove_all", error); }
static OValue osiris_host_root_rename(OValue root, OValue old_path, OValue new_path) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *old_w = NULL, *new_w = NULL; if (!resource || !osiris_win_root_path(resource, old_path, &old_w) || !osiris_win_root_path(resource, new_path, &new_w)) { free(old_w); free(new_w); return ofail("root_rename paths must be relative and confined"); } BOOL ok = MoveFileExW(old_w, new_w, MOVEFILE_REPLACE_EXISTING); DWORD error = GetLastError(); free(old_w); free(new_w); return ok ? obool(1) : osiris_win_error("root_rename", error); }
static OValue osiris_host_root_chmod(OValue root, OValue path, OValue mode) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; if (!resource || !osiris_win_root_path(resource, path, &wide)) return ofail("root_chmod path must be relative and confined"); int64_t numeric_mode = 0; if (!osiris_win_i64(mode, &numeric_mode)) { free(wide); return ofail("root_chmod mode must be an integer"); } DWORD current = GetFileAttributesW(wide); if (current == INVALID_FILE_ATTRIBUTES) { DWORD error = GetLastError(); free(wide); return osiris_win_error("root_chmod", error); } BOOL writable = (numeric_mode & 0200) != 0; BOOL ok = SetFileAttributesW(wide, writable ? current & ~FILE_ATTRIBUTE_READONLY : current | FILE_ATTRIBUTE_READONLY); DWORD error = GetLastError(); free(wide); return ok ? obool(1) : osiris_win_error("root_chmod", error); }

static OValue osiris_host_root_chown(OValue root, OValue path, OValue uid, OValue gid, int link) {
  OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; int64_t user = 0, group = 0;
  if (!resource || !osiris_win_root_path(resource, path, &wide) || !osiris_win_i64(uid, &user) || !osiris_win_i64(gid, &group) || user < -1 || group < -1) { free(wide); return ofail("root_chown expects a confined path and uid/gid integers"); }
  (void)link;
  if (user == -1 && group == -1) { free(wide); return obool(1); }
  free(wide); return oerrorvalue("unsupported", "Windows native C has no POSIX uid/gid ownership model; use -1 for unchanged owners");
}

static int osiris_win_timestamp(OValue value, FILETIME *result) {
  const char *text = NULL; size_t length = 0; if (!osiris_win_string(value, &text, &length) || length < 20) return 0;
  unsigned year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0; if (sscanf(text, "%4u-%2u-%2uT%2u:%2u:%2u", &year, &month, &day, &hour, &minute, &second) != 6 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59) return 0;
  SYSTEMTIME system_time; memset(&system_time, 0, sizeof(system_time)); system_time.wYear = (WORD)year; system_time.wMonth = (WORD)month; system_time.wDay = (WORD)day; system_time.wHour = (WORD)hour; system_time.wMinute = (WORD)minute; system_time.wSecond = (WORD)second;
  return SystemTimeToFileTime(&system_time, result) != 0;
}

static OValue osiris_host_root_chtimes(OValue root, OValue path, OValue access, OValue modification) {
  OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); wchar_t *wide = NULL; FILETIME access_time, modification_time;
  if (!resource || !osiris_win_root_path(resource, path, &wide) || !osiris_win_timestamp(access, &access_time) || !osiris_win_timestamp(modification, &modification_time)) { free(wide); return ofail("root_chtimes expects a confined path and RFC3339 timestamps"); }
  HANDLE handle = CreateFileW(wide, FILE_WRITE_ATTRIBUTES, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL); DWORD error = GetLastError(); free(wide); if (handle == INVALID_HANDLE_VALUE) return osiris_win_error("root_chtimes", error);
  BOOL ok = SetFileTime(handle, NULL, &access_time, &modification_time); error = GetLastError(); CloseHandle(handle); return ok ? obool(1) : osiris_win_error("root_chtimes", error);
}

static OValue osiris_host_root_fs(OValue root) { OHostResource *resource = osiris_win_resource_get(root, OSIRIS_HOST_ROOT); return resource ? osiris_win_resource_value(resource) : onull(); }
static OValue osiris_host_root_close(OValue value) { OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_ROOT); if (!resource) return onull(); osiris_win_resource_close(resource); return obool(1); }

static OValue osiris_host_watch(OValue path_value, OValue recursive_value) { const char *data = NULL; size_t length = 0; if (!osiris_win_string(path_value, &data, &length) || !length) return ofail("watch expects a non-empty path"); int recursive = 1; if (recursive_value.kind != O_NULL) { if (recursive_value.kind != O_BOOL) return ofail("watch recursive argument must be bool"); recursive = recursive_value.as.b != 0; } wchar_t *wide = NULL, *absolute = NULL; if (!osiris_win_utf8_to_wide(data, length, &wide) || !osiris_win_absolute_path(wide, &absolute)) { free(wide); free(absolute); return ofail("watch expects a valid path"); } HANDLE directory = CreateFileW(absolute, FILE_LIST_DIRECTORY, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, NULL, OPEN_EXISTING, FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OVERLAPPED, NULL); DWORD error = GetLastError(); free(wide); if (directory == INVALID_HANDLE_VALUE) { free(absolute); return osiris_win_error("watch", error); } HANDLE event = CreateEventW(NULL, TRUE, FALSE, NULL); if (!event) { error = GetLastError(); CloseHandle(directory); free(absolute); return osiris_win_error("watch.event", error); } OHostResource *resource = osiris_win_resource_new(OSIRIS_HOST_WATCH); if (!resource) { CloseHandle(directory); CloseHandle(event); free(absolute); return ofail("out of memory"); } resource->fd = directory; resource->watch_event = event; resource->path = absolute; resource->recursive = recursive; return osiris_win_resource_value(resource); }
static OValue osiris_host_watch_read(OValue value, OValue size_value, OValue timeout_value) { OHostResource *resource = osiris_win_resource_get(value, OSIRIS_HOST_WATCH); if (!resource) return onull(); int64_t requested = 0, timeout = -1; if (!osiris_win_i64(size_value, &requested) || requested < 0 || requested > OSIRIS_HOST_MAX_IO || (timeout_value.kind != O_NULL && (!osiris_win_i64(timeout_value, &timeout) || timeout < 0))) return ofail("watch_read expects bounded size and timeout"); size_t size = (size_t)requested; while (resource->watch_pending_offset >= resource->watch_pending_length) { OVERLAPPED overlapped; memset(&overlapped, 0, sizeof(overlapped)); overlapped.hEvent = resource->watch_event; ResetEvent(resource->watch_event); DWORD bytes = 0; BOOL started = ReadDirectoryChangesW(resource->fd, resource->watch_buffer, sizeof(resource->watch_buffer), resource->recursive, FILE_NOTIFY_CHANGE_FILE_NAME | FILE_NOTIFY_CHANGE_DIR_NAME | FILE_NOTIFY_CHANGE_SIZE | FILE_NOTIFY_CHANGE_LAST_WRITE, &bytes, &overlapped, NULL); if (!started) return osiris_win_error("watch_read", GetLastError()); DWORD wait = WaitForSingleObject(resource->watch_event, timeout < 0 || timeout > 0xfffffffeLL ? INFINITE : (DWORD)timeout); if (wait == WAIT_TIMEOUT) { CancelIoEx(resource->fd, &overlapped); return oerrorvalue("timeout", "watch_read deadline exceeded"); } if (wait != WAIT_OBJECT_0 || !GetOverlappedResult(resource->fd, &overlapped, &bytes, FALSE)) return osiris_win_error("watch_read", GetLastError()); resource->watch_bytes = bytes; resource->watch_offset = 0; while (resource->watch_offset < bytes) { FILE_NOTIFY_INFORMATION *change = (FILE_NOTIFY_INFORMATION *)(resource->watch_buffer + resource->watch_offset); char *name = osiris_win_wide_to_utf8(change->FileName, change->FileNameLength / sizeof(wchar_t)); if (!name) return ofail("watch event path is not valid UTF-8"); const char *kind = change->Action == FILE_ACTION_ADDED ? "create" : (change->Action == FILE_ACTION_REMOVED ? "remove" : (change->Action == FILE_ACTION_MODIFIED ? "modify" : "rename")); char *root_utf8 = osiris_win_wide_to_utf8(resource->path, wcslen(resource->path)); char *escaped_root = root_utf8 ? osiris_win_json_escape(root_utf8, strlen(root_utf8)) : NULL; char *escaped_name = osiris_win_json_escape(name, strlen(name)); if (!root_utf8 || !escaped_root || !escaped_name) { free(name); free(root_utf8); free(escaped_root); free(escaped_name); return ofail("watch event path is not valid UTF-8"); } size_t event_length = strlen(escaped_root) + strlen(escaped_name) + strlen(kind) + 40; char *event = (char *)malloc(event_length); if (!event) { free(name); return ofail("out of memory"); } char *root_utf8 = osiris_win_wide_to_utf8(resource->path, path_length); if (!root_utf8) { free(name); free(event); return ofail("watch root is not valid UTF-8"); } snprintf(event, event_length, "{\"kind\":\"%s\",\"path\":\"%s\\\\%s\"}\n", kind, escaped_root, escaped_name); free(root_utf8); free(name); free(resource->watch_pending); resource->watch_pending = event; resource->watch_pending_length = strlen(event); resource->watch_pending_offset = 0; resource->watch_offset += change->NextEntryOffset ? change->NextEntryOffset : bytes; break; } }
  size_t available = resource->watch_pending_length - resource->watch_pending_offset; if (available > size) available = size; char *copy = (char *)malloc(available + 1); if (!copy) return ofail("out of memory"); memcpy(copy, resource->watch_pending + resource->watch_pending_offset, available); copy[available] = 0; if (!otrack(copy)) { free(copy); return ofail("out of memory"); } resource->watch_pending_offset += available; if (resource->watch_pending_offset == resource->watch_pending_length) { free(resource->watch_pending); resource->watch_pending = NULL; resource->watch_pending_length = resource->watch_pending_offset = 0; } return ostringview(copy, available);
}

static int osiris_win_wsa(void) { if (osiris_win_wsa_ready) return 1; WSADATA data; if (WSAStartup(MAKEWORD(2, 2), &data) != 0) return 0; osiris_win_wsa_ready = 1; return 1; }
static OValue onet_error(const char *operation, const char *resource, const char *detail) { (void)resource; OValue error = oerrorvalue("network", detail ? detail : operation); olasterror = error; ohaderror = 1; return onull(); }
static int onet_string(OValue value, const char **data, size_t *length, const char *operation) { if (!osiris_win_string(value, data, length) || *length > OSIRIS_NET_MAX_ADDRESS) { onet_error(operation, "", "network address must be bounded UTF-8"); return 0; } return 1; }
static int onet_i64(OValue value, int64_t *out, const char *operation) { if (value.kind == O_NULL) { *out = 0; return 1; } if (!osiris_win_i64(value, out) || *out < 0 || *out > 0x7fffffff) { onet_error(operation, "", "timeout or size must be a non-negative bounded integer"); return 0; } return 1; }
static OValue onet_value(ONetHandle *handle) { return (OValue){O_RESOURCE, 0, {.resource = handle}}; }
static ONetHandle *onet_handle(OValue value, int expected, const char *operation) { if (value.kind != O_RESOURCE || !value.as.resource) { onet_error(operation, "", "expected an open native network resource"); return NULL; } ONetHandle *handle = (ONetHandle *)value.as.resource; if (handle->magic != OSIRIS_NET_MAGIC || handle->closed || handle->fd == INVALID_SOCKET || (expected && handle->kind != expected)) { onet_error(operation, "", "network resource is closed or has the wrong kind"); return NULL; } return handle; }
static ONetHandle *onet_new(SOCKET fd, int kind, const char *operation, const char *resource_name) { (void)operation; (void)resource_name; ONetHandle *handle = (ONetHandle *)calloc(1, sizeof(*handle)); if (!handle) { closesocket(fd); return NULL; } handle->magic = OSIRIS_NET_MAGIC; handle->fd = fd; handle->kind = kind; if (!otrack(handle)) { closesocket(fd); free(handle); return NULL; } return handle; }
static int onet_wait(SOCKET fd, short events, int64_t timeout_ms, const char *operation, const char *resource_name) { WSAPOLLFD descriptor; descriptor.fd = fd; descriptor.events = events; descriptor.revents = 0; int wait_ms = timeout_ms <= 0 ? -1 : (timeout_ms > 0x7fffffff ? 0x7fffffff : (int)timeout_ms); int result = WSAPoll(&descriptor, 1, wait_ms); if (result <= 0) { onet_error(operation, resource_name, result == 0 ? "network operation timed out" : "WSAPoll failed"); return 0; } if (descriptor.revents & (POLLERR | POLLHUP | POLLNVAL)) { onet_error(operation, resource_name, "network peer closed or socket failed"); return 0; } return 1; }
static int onet_split_address(const char *address, char **host, char **service) { if (!address || !host || !service) return 0; *host = NULL; *service = NULL; size_t length = strlen(address); const char *separator = NULL; if (length && address[0] == '[') { const char *closing = strchr(address, ']'); if (!closing || closing[1] != ':') return 0; *host = osiris_win_strdup(address + 1); if (*host) (*host)[closing - address - 1] = 0; separator = closing + 1; } else { separator = strrchr(address, ':'); if (!separator || (strchr(address, ':') != separator && strchr(separator + 1, ':'))) return 0; *host = (char *)malloc((size_t)(separator - address) + 1); if (*host) { memcpy(*host, address, (size_t)(separator - address)); (*host)[separator - address] = 0; } } if (!*host || !separator || !separator[1]) { free(*host); *host = NULL; return 0; } *service = osiris_win_strdup(separator + 1); if (!*service) { free(*host); *host = NULL; return 0; } return 1; }
static int onet_connect_fd(const char *address, int64_t timeout_ms, int type, const char *operation) { if (!osiris_win_wsa()) { onet_error(operation, address, "WSAStartup failed"); return INVALID_SOCKET; } char *host = NULL, *service = NULL; if (!onet_split_address(address, &host, &service)) { onet_error(operation, address, "network address must be host:port or [ipv6]:port"); return INVALID_SOCKET; } struct addrinfo hints, *list = NULL; memset(&hints, 0, sizeof(hints)); hints.ai_family = AF_UNSPEC; hints.ai_socktype = type; int lookup = getaddrinfo(host[0] ? host : NULL, service, &hints, &list); free(host); free(service); if (lookup != 0) { onet_error(operation, address, gai_strerrorA(lookup)); return INVALID_SOCKET; } SOCKET socket_fd = INVALID_SOCKET; for (struct addrinfo *item = list; item; item = item->ai_next) { socket_fd = socket(item->ai_family, item->ai_socktype, item->ai_protocol); if (socket_fd == INVALID_SOCKET) continue; u_long nonblocking = 1; if (timeout_ms > 0) ioctlsocket(socket_fd, FIONBIO, &nonblocking); int result = connect(socket_fd, item->ai_addr, (int)item->ai_addrlen); if (result == 0) { nonblocking = 0; if (timeout_ms > 0) ioctlsocket(socket_fd, FIONBIO, &nonblocking); break; } if (timeout_ms > 0 && WSAGetLastError() == WSAEWOULDBLOCK && onet_wait(socket_fd, POLLOUT, timeout_ms, operation, address)) { int error = 0; int error_length = sizeof(error); getsockopt(socket_fd, SOL_SOCKET, SO_ERROR, (char *)&error, &error_length); if (!error) { nonblocking = 0; ioctlsocket(socket_fd, FIONBIO, &nonblocking); break; } } closesocket(socket_fd); socket_fd = INVALID_SOCKET; } freeaddrinfo(list); if (socket_fd == INVALID_SOCKET) onet_error(operation, address, "unable to connect to network address"); return socket_fd; }
static OValue onet_tcp_dial(OValue address, OValue timeout) { const char *text = NULL; size_t length = 0; if (!onet_string(address, &text, &length)) return onull(); int64_t wait = 0; if (!onet_i64(timeout, &wait, "tcp_dial")) return onull(); SOCKET socket_fd = onet_connect_fd(text, wait, SOCK_STREAM, "tcp_dial"); if (socket_fd == INVALID_SOCKET) return onull(); ONetHandle *handle = onet_new(socket_fd, OSIRIS_NET_TCP, "tcp_dial", text); return handle ? onet_value(handle) : onull(); }
static OValue onet_tcp_listen(OValue address) { const char *text = NULL; size_t length = 0; char *host = NULL, *service = NULL; if (!onet_string(address, &text, &length) || !onet_split_address(text, &host, &service) || !osiris_win_wsa()) return onet_error("tcp_listen", text, "address must be host:port and Winsock must be available"); struct addrinfo hints, *list = NULL; memset(&hints, 0, sizeof(hints)); hints.ai_family = AF_UNSPEC; hints.ai_socktype = SOCK_STREAM; hints.ai_flags = AI_PASSIVE; int lookup = getaddrinfo(host, service, &hints, &list); free(host); free(service); if (lookup != 0) return onet_error("tcp_listen", text, gai_strerrorA(lookup)); SOCKET socket_fd = INVALID_SOCKET; for (struct addrinfo *item = list; item; item = item->ai_next) { socket_fd = socket(item->ai_family, item->ai_socktype, item->ai_protocol); if (socket_fd == INVALID_SOCKET) continue; char yes = 1; setsockopt(socket_fd, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof(yes)); if (bind(socket_fd, item->ai_addr, (int)item->ai_addrlen) == 0 && listen(socket_fd, 128) == 0) break; closesocket(socket_fd); socket_fd = INVALID_SOCKET; } freeaddrinfo(list); if (socket_fd == INVALID_SOCKET) return onet_error("tcp_listen", text, "unable to bind or listen"); ONetHandle *handle = onet_new(socket_fd, OSIRIS_NET_TCP_LISTENER, "tcp_listen", text); return handle ? onet_value(handle) : onull(); }
static OValue onet_tcp_accept(OValue value, OValue timeout) { ONetHandle *listener = onet_handle(value, OSIRIS_NET_TCP_LISTENER, "tcp_accept"); if (!listener) return onull(); int64_t wait = 0; if (!onet_i64(timeout, &wait, "tcp_accept") || !onet_wait(listener->fd, POLLIN, wait, "tcp_accept", "listener")) return onull(); SOCKET accepted = accept(listener->fd, NULL, NULL); if (accepted == INVALID_SOCKET) return onet_error("tcp_accept", "listener", "accept failed"); ONetHandle *handle = onet_new(accepted, OSIRIS_NET_TCP, "tcp_accept", "accepted peer"); return handle ? onet_value(handle) : onull(); }
static OValue onet_read(OValue value, OValue size_value, OValue timeout_value) { ONetHandle *handle = onet_handle(value, OSIRIS_NET_TCP, "readchunk"); if (!handle) return onull(); int64_t size = 0, wait = 0; if (!onet_i64(size_value, &size, "readchunk") || size < 0 || size > OSIRIS_NET_MAX_PACKET || !onet_i64(timeout_value, &wait, "readchunk") || !onet_wait(handle->fd, POLLIN, wait, "readchunk", "tcp")) return onull(); unsigned char *data = (unsigned char *)malloc(size ? (size_t)size : 1); if (size && !data) return ofail("out of memory"); int count = recv(handle->fd, (char *)data, (int)size, 0); if (count < 0) { free(data); return onet_error("readchunk", "tcp", "recv failed"); } OValue result = osiris_win_bytes(data, (size_t)count); free(data); return result; }
static OValue onet_write(OValue value, OValue data_value, OValue timeout_value) { ONetHandle *handle = onet_handle(value, OSIRIS_NET_TCP, "writechunk"); if (!handle) return onull(); unsigned char *data = NULL; size_t length = 0; if (!osiris_win_value_bytes(data_value, &data, &length) || length > OSIRIS_NET_MAX_PACKET) return onet_error("writechunk", "tcp", "write data is not bounded bytes"); int64_t wait = 0; if (!onet_i64(timeout_value, &wait, "writechunk")) { free(data); return onull(); } size_t sent = 0; while (sent < length) { if (!onet_wait(handle->fd, POLLOUT, wait, "writechunk", "tcp")) { free(data); return onull(); } int count = send(handle->fd, (const char *)data + sent, (int)(length - sent > INT_MAX ? INT_MAX : length - sent), 0); if (count <= 0) { free(data); return onet_error("writechunk", "tcp", "send failed"); } sent += (size_t)count; } free(data); return ouintbits(sent, 64); }
static OValue onet_udp_bind(OValue address) { const char *text = NULL; size_t length = 0; if (!onet_string(address, &text, &length)) return onull(); char *host = NULL, *service = NULL; if (!onet_split_address(text, &host, &service) || !osiris_win_wsa()) { free(host); free(service); return onet_error("udp_bind", text, "address must be host:port and Winsock must be available"); } struct addrinfo hints, *list = NULL; memset(&hints, 0, sizeof(hints)); hints.ai_family = AF_UNSPEC; hints.ai_socktype = SOCK_DGRAM; hints.ai_flags = AI_PASSIVE; int lookup = getaddrinfo(host[0] ? host : NULL, service, &hints, &list); free(host); free(service); if (lookup != 0) return onet_error("udp_bind", text, gai_strerrorA(lookup)); SOCKET socket_fd = INVALID_SOCKET; for (struct addrinfo *item = list; item; item = item->ai_next) { socket_fd = socket(item->ai_family, item->ai_socktype, item->ai_protocol); if (socket_fd != INVALID_SOCKET && bind(socket_fd, item->ai_addr, (int)item->ai_addrlen) == 0) break; if (socket_fd != INVALID_SOCKET) closesocket(socket_fd); socket_fd = INVALID_SOCKET; } freeaddrinfo(list); if (socket_fd == INVALID_SOCKET) return onet_error("udp_bind", text, "unable to bind UDP socket"); ONetHandle *handle = onet_new(socket_fd, OSIRIS_NET_UDP, "udp_bind", text); return handle ? onet_value(handle) : onull(); }
static OValue onet_udp_connect(OValue address) { const char *text = NULL; size_t length = 0; if (!onet_string(address, &text, &length)) return onull(); SOCKET socket_fd = onet_connect_fd(text, 0, SOCK_DGRAM, "udp_connect"); if (socket_fd == INVALID_SOCKET) return onull(); ONetHandle *handle = onet_new(socket_fd, OSIRIS_NET_UDP, "udp_connect", text); return handle ? onet_value(handle) : onull(); }
static OValue onet_udp_send(OValue value, OValue data_value, OValue peer_value) { ONetHandle *handle = onet_handle(value, OSIRIS_NET_UDP, "udp_send"); if (!handle) return onull(); unsigned char *data = NULL; size_t length = 0; if (!osiris_win_value_bytes(data_value, &data, &length) || length > OSIRIS_NET_MAX_PACKET) return onet_error("udp_send", "udp", "packet data is not bounded bytes"); int count = -1; if (peer_value.kind == O_NULL) count = send(handle->fd, (const char *)data, (int)length, 0); else { const char *peer = NULL; size_t peer_length = 0; char *host = NULL, *service = NULL; if (!onet_string(peer_value, &peer, &peer_length) || !onet_split_address(peer, &host, &service)) { free(data); return onet_error("udp_send", "udp", "peer must be host:port"); } struct addrinfo hints, *list = NULL; memset(&hints, 0, sizeof(hints)); hints.ai_family = AF_UNSPEC; hints.ai_socktype = SOCK_DGRAM; int lookup = getaddrinfo(host, service, &hints, &list); free(host); free(service); if (!lookup && list) count = sendto(handle->fd, (const char *)data, (int)length, 0, list->ai_addr, (int)list->ai_addrlen); freeaddrinfo(list); } free(data); return count < 0 ? onet_error("udp_send", "udp", "send failed") : ouintbits((uint64_t)count, 64); }
static OValue onet_udp_receive(OValue value, OValue size_value, OValue timeout_value) { ONetHandle *handle = onet_handle(value, OSIRIS_NET_UDP, "udp_receive"); if (!handle) return onull(); int64_t size = OSIRIS_NET_MAX_PACKET, wait = 0; if (size_value.kind != O_NULL && (!onet_i64(size_value, &size, "udp_receive") || size > OSIRIS_NET_MAX_PACKET)) return onull(); if (!onet_i64(timeout_value, &wait, "udp_receive") || !onet_wait(handle->fd, POLLIN, wait, "udp_receive", "udp")) return onull(); unsigned char *data = (unsigned char *)malloc(size ? (size_t)size : 1); if (size && !data) return ofail("out of memory"); struct sockaddr_storage peer; int peer_length = sizeof(peer); int count = recvfrom(handle->fd, (char *)data, (int)size, 0, (struct sockaddr *)&peer, &peer_length); if (count < 0) { free(data); return onet_error("udp_receive", "udp", "receive failed"); } char peer_text[NI_MAXHOST + NI_MAXSERV + 4]; getnameinfo((struct sockaddr *)&peer, peer_length, peer_text, NI_MAXHOST, NULL, 0, NI_NUMERICHOST); OValue bytes = osiris_win_bytes(data, (size_t)count); free(data); OValue values[3] = {bytes, ostring(peer_text), obool(0)}; return olist(values, 3); }
static OValue onet_dns_lookup(OValue host_value, OValue policy_value) { (void)policy_value; const char *host = NULL; size_t length = 0; if (!onet_string(host_value, &host, &length) || !osiris_win_wsa()) return onull(); struct addrinfo hints, *list = NULL; memset(&hints, 0, sizeof(hints)); hints.ai_family = AF_UNSPEC; hints.ai_socktype = SOCK_STREAM; int lookup = getaddrinfo(host, NULL, &hints, &list); if (lookup) return onet_error("dns_lookup", host, gai_strerrorA(lookup)); OValue *items = NULL; size_t count = 0; for (struct addrinfo *item = list; item && count < 256; item = item->ai_next) { char text[NI_MAXHOST]; if (getnameinfo(item->ai_addr, (int)item->ai_addrlen, text, sizeof(text), NULL, 0, NI_NUMERICHOST) == 0) { OValue *grown = (OValue *)realloc(items, (count + 1) * sizeof(OValue)); if (!grown) { free(items); freeaddrinfo(list); return ofail("out of memory"); } items = grown; items[count++] = ostring(text); } } freeaddrinfo(list); return olist(items, count); }
static OValue onet_close(OValue value) { ONetHandle *handle = (ONetHandle *)(value.kind == O_RESOURCE ? value.as.resource : NULL); if (!handle || handle->magic != OSIRIS_NET_MAGIC) return onet_error("close", "", "resource is not a native network resource"); if (handle->closed) return obool(0); int result = closesocket(handle->fd); handle->fd = INVALID_SOCKET; handle->closed = 1; return result == 0 ? obool(1) : onet_error("close", "network", "closesocket failed"); }
static OValue osiris_host_close_any(OValue value) { if (value.kind != O_RESOURCE || !value.as.resource) return ofail("close expects an owned native resource"); uint64_t magic = *((const uint64_t *)value.as.resource); if (magic == OSIRIS_NET_MAGIC) return onet_close(value); OHostResource *resource = (OHostResource *)value.as.resource; if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) return ofail("resource is closed or invalid"); osiris_win_resource_close(resource); return obool(1); }

static void osiris_win_atexit(void) { osiris_win_resource_cleanup(); if (osiris_win_wsa_ready) WSACleanup(); }
static void osiris_win_install_atexit(void) { if (!osiris_win_resources_atexit) { atexit(osiris_win_atexit); osiris_win_resources_atexit = 1; } }
#else
#define OSIRIS_NATIVE_POSIX_NET 0
#endif
`
