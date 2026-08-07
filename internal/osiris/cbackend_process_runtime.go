package osiris

import "strings"

// nativeHostResourceMembers is the authoritative host-resource capability
// table used by nativeStaticModuleMember, nativeModuleCapability, and the
// direct resource dispatcher. Every admitted member must have a matching
// symbol in nativeHostResourceRuntimeComponent and focused execution evidence.
var nativeHostResourceMembers = map[string]map[string]string{
	"std/os": {
		"process_spawn":      "os.process_spawn",
		"start_process":      "os.process_spawn",
		"find_process":       "os.process_inspect",
		"process_wait":       "os.process",
		"process_kill":       "os.process_kill",
		"process_signal":     "os.process_signal",
		"process_release":    "os.process_inspect",
		"process_state":      "os.process_inspect",
		"process_stdin":      "os.process",
		"process_stdout":     "os.process",
		"process_stderr":     "os.process",
		"pipe":               "os.file_read",
		"stdin":              "os.file_read",
		"stdout":             "os.file_write",
		"stderr":             "os.file_write",
		"fd":                 "os.process_raw_handle",
		"name":               "os.file_metadata",
		"new_file":           "os.process_raw_handle",
		"with_handle":        "os.process_raw_handle",
		"syscall_conn":       "os.process_raw_handle",
		"signal_subscribe":   "os.signal",
		"open":               "os.file_read",
		"create":             "os.file_write",
		"open_file":          "os.file_read",
		"stat":               "os.file_metadata",
		"lstat":              "os.file_metadata",
		"read_dir":           "os.directory_read",
		"close":              "os.file_read",
		"read":               "os.file_read",
		"write":              "os.file_write",
		"read_at":            "os.file_read",
		"write_at":           "os.file_write",
		"seek_file":          "os.file_read",
		"sync":               "os.file_write",
		"set_deadline":       "os.file_metadata",
		"set_read_deadline":  "os.file_metadata",
		"set_write_deadline": "os.file_metadata",
		"open_root":          "os.file_read",
		"root_create":        "os.file_write",
		"root_open":          "os.file_read",
		// Native C uses a conservative write grant because flags are runtime
		// values and this ABI cannot prove that root_open_file is read-only.
		"root_open_file":  "os.file_write",
		"root_read_file":  "os.file_read",
		"root_write_file": "os.file_write",
		"root_stat":       "os.file_metadata",
		"root_lstat":      "os.file_metadata",
		"root_chown":      "os.file_mutate",
		"root_lchown":     "os.file_mutate",
		"root_chtimes":    "os.file_mutate",
		"root_fs":         "os.file_read",
		"root_readlink":   "os.symlink",
		"root_link":       "os.file_mutate",
		"root_symlink":    "os.symlink",
		"root_mkdir":      "os.directory_mutate",
		"root_mkdir_all":  "os.directory_mutate",
		"root_remove":     "os.directory_mutate",
		"root_remove_all": "os.directory_mutate",
		"root_rename":     "os.file_mutate",
		"root_chmod":      "os.file_mutate",
		"root_close":      "os.file_read",
		"mkdir":           "os.file_mutate",
		"mkdir_all":       "os.file_mutate",
		"remove":          "os.file_mutate",
		"remove_all":      "os.file_mutate",
		"truncate":        "os.file_mutate",
		"rename":          "os.file_mutate",
		"chmod":           "os.file_mutate",
		"readlink":        "os.symlink",
		"link":            "os.file_mutate",
		"symlink":         "os.symlink",
		"write_string":    "os.file_write",
		"watch":           "os.watch",
	},
}

// nativeHostResourceCapability returns the required explicit grant for a
// resource operation. It remains separate from nativeStaticModuleMember so
// capability policy and public-member admission can be audited independently.
func nativeHostResourceCapability(module, member string) (string, bool) {
	members, ok := nativeHostResourceMembers[module]
	if !ok {
		return "", false
	}
	capability, ok := members[member]
	return capability, ok
}

// nativeHostResourceRuntimeComponent is appended after cRuntimePrelude by the
// native emitter. It uses O_RESOURCE as the opaque resource carrier because that
// kind already has a pointer-sized payload in the generated ABI. The resource
// kind and magic value prevent a file/process/root handle from being confused
// with a dynamic FFI library.
//
// The component is POSIX-only by design. Windows integration must provide the
// same function names with CreateProcess/WaitForSingleObject/HANDLE ownership;
// it must not reinterpret POSIX descriptors as Windows handles.
const nativeHostResourceRuntimeComponent = `
#if !defined(OSIRIS_FORCE_NONPOSIX) && (defined(__unix__) || defined(__APPLE__))
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

extern char **environ;

#define OSIRIS_HOST_RESOURCE_MAGIC UINT32_C(0x4f485253)
#define OSIRIS_HOST_MAX_IO (1024u * 1024u)

typedef enum {
  OSIRIS_HOST_PROCESS = 1,
  OSIRIS_HOST_FILE = 2,
  OSIRIS_HOST_ROOT = 3,
  OSIRIS_HOST_STREAM = 4,
  OSIRIS_HOST_WATCH = 5,
  OSIRIS_HOST_SIGNAL = 6
} OHostResourceKind;

typedef struct OHostResource OHostResource;
typedef struct OHostWatchEntry OHostWatchEntry;
struct OHostWatchEntry { char *path; off_t size; time_t modified; mode_t mode; };
struct OHostResource {
  uint32_t magic;
  uint16_t kind;
  uint16_t closed;
  int fd;
  int input_fd;
  int output_fd;
  int error_fd;
  pid_t pid;
  pid_t process_group;
  pid_t process_session;
  int group_owned;
  int session_owned;
  int waited;
  int wait_status;
  int64_t deadline_ms;
  char *path;
  int recursive;
  OHostWatchEntry *watch_entries;
  size_t watch_count;
  char *watch_pending;
  size_t watch_pending_length;
  size_t watch_pending_offset;
  int signal_number;
  int signal_write_fd;
  OHostResource *next;
};

static OHostResource *osiris_host_resources = NULL;
static int osiris_host_resource_atexit = 0;
static int osiris_host_signal_write_fd = -1;
static int osiris_host_signal_registered_number = 0;
static struct sigaction osiris_host_previous_signal_action;
static int osiris_host_previous_signal_valid = 0;

static void osiris_host_signal_handler(int signal_number) {
  if (osiris_host_signal_write_fd < 0) return;
  unsigned char value = (unsigned char)signal_number;
  (void)write(osiris_host_signal_write_fd, &value, 1);
}

static int64_t osiris_host_now_ms(void) {
  struct timespec ts;
  if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) return 0;
  return (int64_t)ts.tv_sec * 1000 + (int64_t)(ts.tv_nsec / 1000000);
}

static OValue osiris_host_errno(const char *operation) {
  char message[256];
  const char *detail = strerror(errno);
  if (!detail) detail = "host operation failed";
  snprintf(message, sizeof(message), "%s: %s", operation, detail);
  size_t length = strlen(message); char *copy = (char *)malloc(length + 1); if (!copy) return ofail("out of memory"); memcpy(copy, message, length + 1); if (!otrack(copy)) { free(copy); return ofail("out of memory"); } return oerrorvalue("io", copy);
}

static void osiris_host_close_fd(int *fd) {
  if (!fd || *fd < 0) return;
  int saved = errno;
  while (close(*fd) != 0 && errno == EINTR) {}
  *fd = -1;
  errno = saved;
}

static int osiris_host_process_signal_group(OHostResource *resource, int signal_number) {
  if (!resource || resource->pid <= 0) { errno = ESRCH; return -1; }
  if (resource->group_owned && resource->process_group > 0) {
    if (kill(-resource->process_group, signal_number) == 0) return 0;
    if (errno != ESRCH) return -1;
  }
  return kill(resource->pid, signal_number);
}

static void osiris_host_process_reap(OHostResource *resource) {
  if (!resource || resource->waited || resource->pid <= 0) return;
  for (;;) {
    pid_t result = waitpid(resource->pid, &resource->wait_status, 0);
    if (result == resource->pid) { resource->waited = 1; return; }
    if (result < 0 && errno == EINTR) continue;
    if (result < 0 && errno == ECHILD) { resource->waited = 1; return; }
    return;
  }
}

static void osiris_host_process_release_group(OHostResource *resource) {
  if (!resource || !resource->group_owned) return;
  (void)osiris_host_process_signal_group(resource, SIGKILL);
  resource->group_owned = 0;
  resource->session_owned = 0;
}

static void osiris_host_process_escalate(OHostResource *resource) {
  if (!resource || resource->waited) {
    osiris_host_process_release_group(resource);
    return;
  }
  (void)osiris_host_process_signal_group(resource, SIGTERM);
  int64_t deadline = osiris_host_now_ms() + 100;
  while (!resource->waited && osiris_host_now_ms() < deadline) {
    pid_t result = waitpid(resource->pid, &resource->wait_status, WNOHANG);
    if (result == resource->pid || (result < 0 && errno == ECHILD)) { resource->waited = 1; break; }
    if (result < 0 && errno != EINTR) break;
    struct timespec pause_for = {.tv_sec=0, .tv_nsec=1000000L};
    nanosleep(&pause_for, NULL);
  }
  if (!resource->waited) (void)osiris_host_process_signal_group(resource, SIGKILL);
  osiris_host_process_reap(resource);
  osiris_host_process_release_group(resource);
}

static void osiris_host_resource_close(OHostResource *resource) {
  if (!resource || resource->closed) return;
  if (resource->kind == OSIRIS_HOST_FILE || resource->kind == OSIRIS_HOST_ROOT || resource->kind == OSIRIS_HOST_STREAM) {
    osiris_host_close_fd(&resource->fd);
  } else if (resource->kind == OSIRIS_HOST_PROCESS) {
    if (resource->group_owned || resource->session_owned) {
      if (!resource->waited) osiris_host_process_escalate(resource);
      else osiris_host_process_release_group(resource);
    }
    osiris_host_close_fd(&resource->input_fd);
    osiris_host_close_fd(&resource->output_fd);
    osiris_host_close_fd(&resource->error_fd);
  } else if (resource->kind == OSIRIS_HOST_SIGNAL) {
    if (osiris_host_signal_write_fd == resource->signal_write_fd) {
      osiris_host_signal_write_fd = -1;
      osiris_host_signal_registered_number = 0;
      if (osiris_host_previous_signal_valid) {
        (void)sigaction(resource->signal_number, &osiris_host_previous_signal_action, NULL);
        osiris_host_previous_signal_valid = 0;
      }
    }
    osiris_host_close_fd(&resource->fd);
    osiris_host_close_fd(&resource->signal_write_fd);
  }
  if (resource->kind == OSIRIS_HOST_WATCH) {
    if (resource->watch_entries) { for (size_t i = 0; i < resource->watch_count; i++) free(resource->watch_entries[i].path); free(resource->watch_entries); resource->watch_entries = NULL; resource->watch_count = 0; }
    free(resource->watch_pending); resource->watch_pending = NULL; resource->watch_pending_length = 0; resource->watch_pending_offset = 0;
    free(resource->path); resource->path = NULL;
  }
  if (resource->kind != OSIRIS_HOST_WATCH) { free(resource->path); resource->path = NULL; }
  resource->closed = 1;
}

static void osiris_host_resource_cleanup(void) {
  OHostResource *resource = osiris_host_resources;
  while (resource) {
    osiris_host_resource_close(resource);
    resource = resource->next;
  }
}

static OHostResource *osiris_host_resource_new(OHostResourceKind kind) {
  OHostResource *resource = (OHostResource *)calloc(1, sizeof(*resource));
  if (!resource) return NULL;
  resource->magic = OSIRIS_HOST_RESOURCE_MAGIC;
  resource->kind = (uint16_t)kind;
  resource->fd = -1;
  resource->input_fd = -1;
  resource->output_fd = -1;
  resource->error_fd = -1;
  resource->signal_write_fd = -1;
  resource->pid = (pid_t)-1;
  resource->deadline_ms = -1;
  resource->next = osiris_host_resources;
  osiris_host_resources = resource;
  if (!osiris_host_resource_atexit) {
    atexit(osiris_host_resource_cleanup);
    osiris_host_resource_atexit = 1;
  }
  return resource;
}

static OValue osiris_host_resource_value(OHostResource *resource) {
  if (!resource) return ofail("out of memory");
  return (OValue){O_RESOURCE, (uint16_t)resource->kind, {.resource=(void *)resource}};
}

static OHostResource *osiris_host_resource_get(OValue value, OHostResourceKind kind, const char *operation) {
  if (value.kind != O_RESOURCE || !value.as.resource) {
    ofail("host resource handle expected");
    return NULL;
  }
  OHostResource *resource = (OHostResource *)value.as.resource;
  if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->kind != (uint16_t)kind || resource->closed) {
    ofail("host resource is closed or has the wrong kind");
    return NULL;
  }
  (void)operation;
  return resource;
}

static int osiris_host_wait_fd(int fd, short events, int64_t timeout_ms) {
  if (fd < 0) { errno = EBADF; return -1; }
  int timeout = -1;
  if (timeout_ms >= 0) {
    if (timeout_ms > INT_MAX) timeout = INT_MAX;
    else timeout = (int)timeout_ms;
  }
  struct pollfd pfd = {.fd=fd, .events=events, .revents=0};
  for (;;) {
    int ready = poll(&pfd, 1, timeout);
    if (ready >= 0) {
      if (ready == 0) { errno = ETIMEDOUT; return 0; }
      if (pfd.revents & (POLLERR | POLLNVAL)) { errno = EIO; return -1; }
      return 1;
    }
    if (errno != EINTR) return -1;
  }
}

static int osiris_host_effective_timeout(OHostResource *resource, OValue timeout) {
  int64_t requested = -1;
  if (timeout.kind == O_INT) requested = timeout.as.i;
  else if (timeout.kind == O_UINT && timeout.as.u <= INT64_MAX) requested = (int64_t)timeout.as.u;
  else if (timeout.kind != O_NULL) { errno = EINVAL; return -2; }
  if (requested < -1) { errno = EINVAL; return -2; }
  if (resource && resource->deadline_ms >= 0) {
    int64_t remaining = resource->deadline_ms - osiris_host_now_ms();
    if (remaining < 0) remaining = 0;
    if (requested < 0 || remaining < requested) requested = remaining;
  }
  return requested > INT_MAX ? INT_MAX : (int)requested;
}

static int osiris_host_string(OValue value, const char **data, size_t *length) {
  if (value.kind != O_STRING || !value.as.s.data || value.as.s.len > SIZE_MAX - 1) return 0;
  if (memchr(value.as.s.data, 0, value.as.s.len) != NULL) return 0;
  *data = value.as.s.data;
  *length = value.as.s.len;
  return 1;
}

static int osiris_host_argv(OValue value, char ***out, size_t *count) {
  if (value.kind != O_LIST || value.as.list.len > SIZE_MAX / sizeof(char *) - 1) return 0;
  size_t n = value.as.list.len;
  char **argv = (char **)calloc(n + 1, sizeof(char *));
  if (!argv) return 0;
  for (size_t i = 0; i < n; i++) {
    const char *data = NULL; size_t length = 0;
    if (!osiris_host_string(value.as.list.items[i], &data, &length)) {
      for (size_t j = 0; j < i; j++) free(argv[j]);
      free(argv); return 0;
    }
    argv[i] = (char *)malloc(length + 1);
    if (!argv[i]) { for (size_t j = 0; j < i; j++) free(argv[j]); free(argv); return 0; }
    memcpy(argv[i], data, length); argv[i][length] = 0;
  }
  *out = argv; *count = n; return 1;
}

static void osiris_host_free_argv(char **argv, size_t count) {
  if (!argv) return;
  for (size_t i = 0; i < count; i++) free(argv[i]);
  free(argv);
}

static int osiris_host_string_compare(const void *left, const void *right) {
  const char *const *a = (const char *const *)left;
  const char *const *b = (const char *const *)right;
  return strcmp(*a, *b);
}

static int osiris_host_environment_key(const char *entry, const char *key, size_t key_length) {
  return entry && key && strncmp(entry, key, key_length) == 0 && entry[key_length] == '=';
}

static int osiris_host_environment_reserve(char ***values, size_t *count, size_t *capacity, size_t needed) {
  if (needed <= *capacity) return 1;
  size_t next = *capacity ? *capacity : 16;
  while (next < needed) {
    if (next > SIZE_MAX / 2) return 0;
    next *= 2;
  }
  char **grown = (char **)realloc(*values, next * sizeof(*grown));
  if (!grown) return 0;
  *values = grown; *capacity = next; return 1;
}

static int osiris_host_environment_set(char ***values, size_t *count, size_t *capacity, const char *key, size_t key_length, const char *value, size_t value_length) {
  for (size_t i = 0; i < *count; i++) {
    if (!osiris_host_environment_key((*values)[i], key, key_length)) continue;
    if (!value) {
      free((*values)[i]);
      memmove(*values + i, *values + i + 1, (*count - i - 1) * sizeof(**values));
      (*count)--;
      return 1;
    }
    if (key_length > SIZE_MAX - value_length - 2) return 0;
    char *replacement = (char *)malloc(key_length + value_length + 2);
    if (!replacement) return 0;
    memcpy(replacement, key, key_length); replacement[key_length] = '=';
    memcpy(replacement + key_length + 1, value, value_length); replacement[key_length + value_length + 1] = 0;
    free((*values)[i]); (*values)[i] = replacement; return 1;
  }
  if (!value) return 1;
  if (key_length > SIZE_MAX - value_length - 2 || !osiris_host_environment_reserve(values, count, capacity, *count + 1)) return 0;
  char *entry = (char *)malloc(key_length + value_length + 2);
  if (!entry) return 0;
  memcpy(entry, key, key_length); entry[key_length] = '=';
  memcpy(entry + key_length + 1, value, value_length); entry[key_length + value_length + 1] = 0;
  (*values)[(*count)++] = entry; return 1;
}

static int osiris_host_environment_copy_inherited(char ***values, size_t *count, size_t *capacity) {
  if (!environ) return 1;
  for (char **entry = environ; *entry; entry++) {
    const char *separator = strchr(*entry, '=');
    if (!separator || separator == *entry) continue;
    size_t key_length = (size_t)(separator - *entry);
    if (!osiris_host_environment_set(values, count, capacity, *entry, key_length, separator + 1, strlen(separator + 1))) return 0;
  }
  return 1;
}

static OValue osiris_host_bytes(const unsigned char *data, size_t length) {
  if (length > OSIRIS_HOST_MAX_IO) return ofail("host I/O result exceeds the native bound");
  if (!length) return olist(NULL, 0);
  OValue *items = (OValue *)malloc(length * sizeof(OValue));
  if (!items) return ofail("out of memory");
  for (size_t i = 0; i < length; i++) items[i] = ouintbits(data[i], 8);
  OValue result = olist(items, length);
  free(items);
  return result;
}

static int osiris_host_value_bytes(OValue value, unsigned char **out, size_t *length) {
  if (!out || !length) return 0;
  *out = NULL; *length = 0;
  if (value.kind == O_STRING) {
    if (!value.as.s.data || value.as.s.len > OSIRIS_HOST_MAX_IO || memchr(value.as.s.data, 0, value.as.s.len)) return 0;
    unsigned char *copy = (unsigned char *)malloc(value.as.s.len ? value.as.s.len : 1);
    if (!copy && value.as.s.len) return 0;
    if (value.as.s.len) memcpy(copy, value.as.s.data, value.as.s.len);
    *out = copy; *length = value.as.s.len; return 1;
  }
  if (value.kind != O_LIST || value.as.list.len > OSIRIS_HOST_MAX_IO) return 0;
  unsigned char *copy = (unsigned char *)malloc(value.as.list.len ? value.as.list.len : 1);
  if (!copy && value.as.list.len) return 0;
  for (size_t i = 0; i < value.as.list.len; i++) {
    OValue item = value.as.list.items[i];
    if (item.kind != O_UINT || item.as.u > 255) { free(copy); return 0; }
    copy[i] = (unsigned char)item.as.u;
  }
  *out = copy; *length = value.as.list.len; return 1;
}

static OValue osiris_host_owned_string(const char *data, size_t length) {
  if (!data || length > OSIRIS_HOST_MAX_IO) return ofail("host string exceeds the native bound");
  char *copy = (char *)malloc(length + 1);
  if (!copy) return ofail("out of memory");
  memcpy(copy, data, length); copy[length] = 0;
  if (!otrack(copy)) { free(copy); return ofail("out of memory"); }
  return ostringview(copy, length);
}

static OValue osiris_host_pipe(void) {
  int descriptors[2] = {-1, -1};
  if (pipe(descriptors) != 0) return osiris_host_errno("pipe");
  OHostResource *read_resource = osiris_host_resource_new(OSIRIS_HOST_STREAM);
  OHostResource *write_resource = osiris_host_resource_new(OSIRIS_HOST_STREAM);
  if (!read_resource || !write_resource) {
    if (read_resource) osiris_host_resource_close(read_resource);
    if (write_resource) osiris_host_resource_close(write_resource);
    osiris_host_close_fd(&descriptors[0]); osiris_host_close_fd(&descriptors[1]);
    return ofail("out of memory");
  }
  read_resource->fd = descriptors[0]; write_resource->fd = descriptors[1];
  read_resource->path = strdup("pipe:read"); write_resource->path = strdup("pipe:write");
  if (!read_resource->path || !write_resource->path) {
    osiris_host_resource_close(read_resource); osiris_host_resource_close(write_resource);
    return ofail("out of memory");
  }
  OValue values[2] = {osiris_host_resource_value(read_resource), osiris_host_resource_value(write_resource)};
  return olist(values, 2);
}

static OValue osiris_host_standard_stream(int which) {
  int descriptor = which == 0 ? STDIN_FILENO : (which == 1 ? STDOUT_FILENO : STDERR_FILENO);
  int duplicate = dup(descriptor);
  if (duplicate < 0) return osiris_host_errno("standard_stream");
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_STREAM);
  if (!resource) { osiris_host_close_fd(&duplicate); return ofail("out of memory"); }
  resource->fd = duplicate;
  const char *name = which == 0 ? "/dev/stdin" : (which == 1 ? "/dev/stdout" : "/dev/stderr");
  resource->path = strdup(name);
  if (!resource->path) { osiris_host_resource_close(resource); return ofail("out of memory"); }
  return osiris_host_resource_value(resource);
}

static OHostResource *osiris_host_any_resource(OValue value, const char *operation) {
  if (value.kind != O_RESOURCE || !value.as.resource) { ofail(operation); return NULL; }
  OHostResource *resource = (OHostResource *)value.as.resource;
  if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) { ofail("host resource is closed or invalid"); return NULL; }
  return resource;
}

static OValue osiris_host_fd(OValue value) {
  OHostResource *resource = osiris_host_any_resource(value, "fd expects a host resource");
  if (!resource || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) return onull();
  return ouint((uint64_t)resource->fd);
}

static OValue osiris_host_with_handle(OValue value, OValue callback) {
  OHostResource *resource = osiris_host_any_resource(value, "with_handle expects a host resource");
  if (!resource || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) return onull();
  if (callback.kind != O_CLOSURE || !callback.as.closure.call) return ofail("with_handle expects a callback");
  OValue raw = ouint((uint64_t)resource->fd);
  return ocall(callback, &raw, 1);
}

static OValue osiris_host_syscall_conn_apply(const OValue *args, size_t argc, const OValue *captures, size_t capture_count) {
  if (argc != 1 || capture_count != 1) return ofail("syscall connection callback arity mismatch");
  OHostResource *resource = osiris_host_any_resource(captures[0], "syscall connection resource is closed");
  if (!resource || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) return onull();
  if (args[0].kind != O_CLOSURE || !args[0].as.closure.call) return ofail("syscall connection method expects a callback");
  OValue raw = ouint((uint64_t)resource->fd);
  return ocall(args[0], &raw, 1);
}

static OValue osiris_host_syscall_conn(OValue value) {
  OHostResource *resource = osiris_host_any_resource(value, "syscall_conn expects a host resource");
  if (!resource || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) return onull();
  OValue methods[3] = {
    oclosure(osiris_host_syscall_conn_apply, &value, 1, "func(func(u64) any) any"),
    oclosure(osiris_host_syscall_conn_apply, &value, 1, "func(func(u64) any) any"),
    oclosure(osiris_host_syscall_conn_apply, &value, 1, "func(func(u64) any) any"),
  };
  if (ohaderror) return onull();
  const char *keys[3] = {"control", "read", "write"};
  return omap(keys, methods, 3);
}

static OValue osiris_host_name(OValue value) {
  OHostResource *resource = osiris_host_any_resource(value, "name expects a host resource");
  if (!resource) return onull();
  const char *name = resource->path ? resource->path : "";
  return osiris_host_owned_string(name, strlen(name));
}

static OValue osiris_host_new_file(OValue fd_value, OValue name_value) {
  int64_t descriptor = fd_value.kind == O_INT ? fd_value.as.i : (fd_value.kind == O_UINT && fd_value.as.u <= INT_MAX ? (int64_t)fd_value.as.u : -1);
  const char *name = NULL; size_t length = 0;
  if (descriptor < 0 || !osiris_host_string(name_value, &name, &length) || length > OSIRIS_HOST_MAX_IO) return ofail("new_file expects a valid descriptor and name");
  int duplicate = dup((int)descriptor);
  if (duplicate < 0) return osiris_host_errno("new_file");
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_FILE);
  if (!resource) { osiris_host_close_fd(&duplicate); return ofail("out of memory"); }
  resource->fd = duplicate; resource->path = (char *)malloc(length + 1);
  if (!resource->path) { osiris_host_resource_close(resource); return ofail("out of memory"); }
  memcpy(resource->path, name, length); resource->path[length] = 0;
  return osiris_host_resource_value(resource);
}

static int osiris_host_signal_number(const char *name, size_t length) {
  if (length == 6 && !memcmp(name, "SIGINT", 6)) return SIGINT;
  if (length == 7 && !memcmp(name, "SIGTERM", 7)) return SIGTERM;
  if (length == 7 && !memcmp(name, "SIGUSR1", 7)) return SIGUSR1;
  if (length == 7 && !memcmp(name, "SIGUSR2", 7)) return SIGUSR2;
  if (length == 5 && !memcmp(name, "SIGHUP", 5)) return SIGHUP;
  return 0;
}

static OValue osiris_host_signal_subscribe(OValue name_value) {
  const char *name = NULL; size_t length = 0;
  if (!osiris_host_string(name_value, &name, &length)) return ofail("signal_subscribe expects a signal name");
  int signal_number = osiris_host_signal_number(name, length);
  if (!signal_number) return ofail("signal_subscribe received an unsupported signal name");
  if (osiris_host_signal_write_fd >= 0) return oerrorvalue("busy", "a native signal subscription is already active");
  int descriptors[2] = {-1, -1};
  if (pipe(descriptors) != 0) return osiris_host_errno("signal_subscribe.pipe");
  int flags = fcntl(descriptors[1], F_GETFL, 0); if (flags >= 0) (void)fcntl(descriptors[1], F_SETFL, flags | O_NONBLOCK);
  struct sigaction action; memset(&action, 0, sizeof(action)); sigemptyset(&action.sa_mask); action.sa_handler = osiris_host_signal_handler;
  if (sigaction(signal_number, &action, &osiris_host_previous_signal_action) != 0) { osiris_host_close_fd(&descriptors[0]); osiris_host_close_fd(&descriptors[1]); return osiris_host_errno("signal_subscribe.sigaction"); }
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_SIGNAL);
  if (!resource) { (void)sigaction(signal_number, &osiris_host_previous_signal_action, NULL); osiris_host_close_fd(&descriptors[0]); osiris_host_close_fd(&descriptors[1]); return ofail("out of memory"); }
  resource->fd = descriptors[0]; resource->signal_write_fd = descriptors[1]; resource->signal_number = signal_number; resource->path = (char *)malloc(length + 1);
  if (!resource->path) { osiris_host_resource_close(resource); return ofail("out of memory"); }
  memcpy(resource->path, name, length); resource->path[length] = 0;
  osiris_host_signal_write_fd = descriptors[1]; osiris_host_signal_registered_number = signal_number; osiris_host_previous_signal_valid = 1;
  return osiris_host_resource_value(resource);
}

static OValue osiris_host_signal_read(OValue value, OValue size_value, OValue timeout_value) {
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_SIGNAL, "signal_read");
  if (!resource) return onull();
  int64_t size = size_value.kind == O_UINT ? (int64_t)size_value.as.u : (size_value.kind == O_INT ? size_value.as.i : -1);
  if (size <= 0) return ofail("signal read size must be positive");
  int timeout = osiris_host_effective_timeout(resource, timeout_value); if (timeout == -2) return osiris_host_errno("signal_read.timeout");
  if (osiris_host_wait_fd(resource->fd, POLLIN, timeout) <= 0) return osiris_host_errno("signal_read.poll");
  unsigned char received = 0; ssize_t count = read(resource->fd, &received, 1);
  if (count != 1) return osiris_host_errno("signal_read");
  return osiris_host_owned_string(resource->path ? resource->path : "", resource->path ? strlen(resource->path) : 0);
}

static OValue osiris_host_process_stream(OValue value, int which);
static OValue osiris_host_process_value(OValue value) {
  OValue *member = omapget(value, "process");
  return member ? *member : value;
}

static int osiris_host_process_options(OValue options, char **cwd, char ***environment, size_t *environment_count, int *has_environment, int *inherit_environment) {
  if (cwd) *cwd = NULL;
  if (environment) *environment = NULL;
  if (environment_count) *environment_count = 0;
  if (has_environment) *has_environment = 0;
  if (inherit_environment) *inherit_environment = 1;
  if (options.kind == O_NULL) return 1;
  if (options.kind != O_MAP) { ofail("process options must be a map or null"); return 0; }
  OValue *cwd_value = omapget(options, "cwd");
  if (cwd_value) {
    const char *data = NULL; size_t length = 0;
    if (!osiris_host_string(*cwd_value, &data, &length) || length == 0) { ofail("process options cwd must be a non-empty NUL-free string"); return 0; }
    if (cwd) { *cwd = (char *)malloc(length + 1); if (!*cwd) { ofail("out of memory"); return 0; } memcpy(*cwd, data, length); (*cwd)[length] = 0; }
  }
  OValue *inherit_value = omapget(options, "inherit_env");
  if (inherit_value) { if (inherit_value->kind != O_BOOL) { free(cwd ? *cwd : NULL); if (cwd) *cwd = NULL; ofail("process options inherit_env must be bool"); return 0; } if (inherit_environment) *inherit_environment = inherit_value->as.b != 0; }
  OValue *environment_value = omapget(options, "env");
  if (!environment_value && inherit_environment && *inherit_environment) return 1;
  if (environment_value && (environment_value->kind != O_MAP || !environment_value->as.map)) { free(cwd ? *cwd : NULL); if (cwd) *cwd = NULL; ofail("process options env must be a map"); return 0; }
  char **values = NULL; size_t count = 0; size_t capacity = 0;
  if (inherit_environment && *inherit_environment && !osiris_host_environment_copy_inherited(&values, &count, &capacity)) goto environment_oom;
  if (environment_value) {
    for (size_t i = 0; i < environment_value->as.map->len; i++) {
      OMapEntry *entry = &environment_value->as.map->entries[i];
      if (entry->key.len == 0 || memchr(entry->key.data, '=', entry->key.len) || memchr(entry->key.data, 0, entry->key.len) || (entry->value.kind != O_STRING && entry->value.kind != O_NULL) || (entry->value.kind == O_STRING && memchr(entry->value.as.s.data, 0, entry->value.as.s.len))) {
        for (size_t j = 0; j < count; j++) free(values[j]); free(values); free(cwd ? *cwd : NULL); if (cwd) *cwd = NULL; ofail("process environment keys must be valid strings and values must be strings or null"); return 0;
      }
      const char *value = entry->value.kind == O_STRING ? entry->value.as.s.data : NULL;
      size_t value_length = entry->value.kind == O_STRING ? entry->value.as.s.len : 0;
      if (!osiris_host_environment_set(&values, &count, &capacity, entry->key.data, entry->key.len, value, value_length)) goto environment_oom;
    }
  }
  if (count > 1) qsort(values, count, sizeof(char *), osiris_host_string_compare);
  if (!osiris_host_environment_reserve(&values, &count, &capacity, count + 1)) goto environment_oom;
  values[count] = NULL;
  if (environment) *environment = values; else { for (size_t i = 0; i < count; i++) free(values[i]); free(values); }
  if (environment_count) *environment_count = count;
  if (has_environment) *has_environment = 1;
  return 1;

environment_oom:
  for (size_t i = 0; i < count; i++) free(values[i]);
  free(values); free(cwd ? *cwd : NULL); if (cwd) *cwd = NULL; ofail("out of memory"); return 0;
}

static void osiris_host_free_environment(char **environment, size_t count) { if (!environment) return; for (size_t i = 0; i < count; i++) free(environment[i]); free(environment); }

static OValue osiris_host_process_spawn(OValue command, OValue arguments, OValue options) {
  const char *command_data = NULL; size_t command_length = 0;
  char **argv = NULL; size_t argument_count = 0;
  if (!osiris_host_string(command, &command_data, &command_length) || command_length == 0) return ofail("process_spawn expects a non-empty NUL-free command");
  if (!osiris_host_argv(arguments, &argv, &argument_count)) return ofail("process_spawn expects a list of NUL-free string arguments");
  char *cwd = NULL; char **environment = NULL; size_t environment_count = 0; int has_environment = 0; int inherit_environment = 1;
  if (!osiris_host_process_options(options, &cwd, &environment, &environment_count, &has_environment, &inherit_environment)) { osiris_host_free_argv(argv, argument_count); return onull(); }
  char *command_copy = (char *)malloc(command_length + 1);
  if (!command_copy) { osiris_host_free_argv(argv, argument_count); return ofail("out of memory"); }
  memcpy(command_copy, command_data, command_length); command_copy[command_length] = 0;
  if (argument_count > SIZE_MAX - 2 || argument_count + 2 > SIZE_MAX / sizeof(*argv)) {
    osiris_host_free_argv(argv, argument_count); free(command_copy); osiris_host_free_environment(environment, environment_count); free(cwd); return ofail("process_spawn argument list is too large");
  }
  char **grown_argv = (char **)realloc(argv, (argument_count + 2) * sizeof(*grown_argv));
  if (!grown_argv) {
    osiris_host_free_argv(argv, argument_count); free(command_copy); osiris_host_free_environment(environment, environment_count); free(cwd); return ofail("out of memory");
  }
  memmove(grown_argv + 1, grown_argv, argument_count * sizeof(*grown_argv));
  grown_argv[0] = command_copy;
  argv = grown_argv;
  argument_count++;
  int input[2] = {-1,-1}, output[2] = {-1,-1}, error[2] = {-1,-1};
  if (pipe(input) || pipe(output) || pipe(error)) {
    osiris_host_close_fd(&input[0]); osiris_host_close_fd(&input[1]);
    osiris_host_close_fd(&output[0]); osiris_host_close_fd(&output[1]);
    osiris_host_close_fd(&error[0]); osiris_host_close_fd(&error[1]);
    osiris_host_free_argv(argv, argument_count); osiris_host_free_environment(environment, environment_count); free(cwd); return osiris_host_errno("process_spawn.pipe");
  }
  pid_t pid = fork();
  if (pid < 0) {
    osiris_host_close_fd(&input[0]); osiris_host_close_fd(&input[1]); osiris_host_close_fd(&output[0]); osiris_host_close_fd(&output[1]); osiris_host_close_fd(&error[0]); osiris_host_close_fd(&error[1]);
    osiris_host_free_argv(argv, argument_count); osiris_host_free_environment(environment, environment_count); free(cwd); return osiris_host_errno("process_spawn.fork");
  }
  if (pid == 0) {
    if (setsid() < 0) _exit(126);
    dup2(input[0], STDIN_FILENO); dup2(output[1], STDOUT_FILENO); dup2(error[1], STDERR_FILENO);
    close(input[0]); close(input[1]); close(output[0]); close(output[1]); close(error[0]); close(error[1]);
    if (cwd && chdir(cwd) != 0) _exit(126);
    argv[argument_count] = NULL;
    if (has_environment) { char *empty_environment[] = {NULL}; execve(command_copy, argv, environment ? environment : empty_environment); }
    else if (!inherit_environment) { char *empty_environment[] = {NULL}; execve(command_copy, argv, empty_environment); }
    else execvp(command_copy, argv);
    _exit(127);
  }
  osiris_host_close_fd(&input[0]); osiris_host_close_fd(&output[1]); osiris_host_close_fd(&error[1]);
  osiris_host_free_argv(argv, argument_count);
  osiris_host_free_environment(environment, environment_count);
  free(cwd);
  command_copy = NULL;
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_PROCESS);
  if (!resource) { (void)kill(-pid, SIGKILL); (void)kill(pid, SIGKILL); waitpid(pid, NULL, 0); osiris_host_close_fd(&input[1]); osiris_host_close_fd(&output[0]); osiris_host_close_fd(&error[0]); return ofail("out of memory"); }
  resource->pid = pid; resource->process_group = pid; resource->process_session = pid; resource->group_owned = 1; resource->session_owned = 1;
  resource->input_fd = input[1]; resource->output_fd = output[0]; resource->error_fd = error[0];
  OValue process = osiris_host_resource_value(resource);
  OValue values[5] = {process, oint((int64_t)pid), osiris_host_process_stream(process, 0), osiris_host_process_stream(process, 1), osiris_host_process_stream(process, 2)};
  const char *keys[5] = {"process", "pid", "stdin", "stdout", "stderr"};
  return omap(keys, values, 5);
}

static OValue osiris_host_process_wait(OValue value, OValue timeout) {
  value = osiris_host_process_value(value);
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_PROCESS, "process_wait");
  if (!resource) return onull();
  int timeout_ms = osiris_host_effective_timeout(resource, timeout);
  if (timeout_ms == -2) return osiris_host_errno("process_wait.timeout");
  int64_t end = timeout_ms < 0 ? -1 : osiris_host_now_ms() + timeout_ms;
  for (;;) {
    pid_t result = waitpid(resource->pid, &resource->wait_status, WNOHANG);
    if (result < 0 && errno == EINTR) continue;
    if (result == resource->pid) {
      resource->waited = 1;
      OValue values[3] = {oint((int64_t)(WIFEXITED(resource->wait_status) ? WEXITSTATUS(resource->wait_status) : 128 + WTERMSIG(resource->wait_status))), obool(WIFEXITED(resource->wait_status) && WEXITSTATUS(resource->wait_status) == 0), ostring(WIFSIGNALED(resource->wait_status) ? "signal" : "")};
      const char *keys[3] = {"status", "success", "signal"};
      return omap(keys, values, 3);
    }
    if (result < 0) return osiris_host_errno("process_wait.waitpid");
    if (end >= 0 && osiris_host_now_ms() >= end) {
      osiris_host_process_escalate(resource);
      return oerrorvalue("timeout", "process_wait deadline exceeded");
    }
    struct timespec pause_for = {.tv_sec=0, .tv_nsec=1000000L};
    nanosleep(&pause_for, NULL);
  }
}

static OValue osiris_host_process_find(OValue pid_value) {
  int64_t pid_number = -1;
  if (pid_value.kind == O_INT) pid_number = pid_value.as.i;
  else if (pid_value.kind == O_UINT && pid_value.as.u <= INT64_MAX) pid_number = (int64_t)pid_value.as.u;
  if (pid_number <= 0 || pid_number > INT_MAX) return ofail("find_process expects a positive process id");
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_PROCESS);
  if (!resource) return ofail("out of memory");
  resource->pid = (pid_t)pid_number;
  return osiris_host_resource_value(resource);
}

static OValue osiris_host_process_state(OValue value) {
  value = osiris_host_process_value(value);
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_PROCESS, "process_state");
  if (!resource) return onull();
  if (!resource->waited) {
    pid_t result = waitpid(resource->pid, &resource->wait_status, WNOHANG);
    if (result < 0 && errno == EINTR) return oerrorvalue("interrupted", "process_state was interrupted while checking the child");
    if (result == resource->pid) resource->waited = 1;
    else if (result == 0) return oerrorvalue("busy", "process_state is not available until the process exits");
    else if (result < 0 && errno != ECHILD) return osiris_host_errno("process_state.waitpid");
    else if (result < 0 && errno == ECHILD) return oerrorvalue("unavailable", "process state is not owned by this process");
  }
  OValue values[3] = {oint((int64_t)(WIFEXITED(resource->wait_status) ? WEXITSTATUS(resource->wait_status) : 128 + WTERMSIG(resource->wait_status))), obool(WIFEXITED(resource->wait_status) && WEXITSTATUS(resource->wait_status) == 0), ostring(WIFSIGNALED(resource->wait_status) ? "signal" : "")};
  const char *keys[3] = {"status", "success", "signal"};
  return omap(keys, values, 3);
}

static OValue osiris_host_process_kill(OValue value, OValue signal_value) {
  value = osiris_host_process_value(value);
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_PROCESS, "process_kill");
  if (!resource) return onull();
  int signal_number = SIGKILL;
  if (signal_value.kind == O_INT && signal_value.as.i >= 0 && signal_value.as.i <= INT_MAX) signal_number = (int)signal_value.as.i;
  else if (signal_value.kind == O_UINT && signal_value.as.u <= INT_MAX) signal_number = (int)signal_value.as.u;
  else if (signal_value.kind != O_NULL) return ofail("process signal must be an integer");
  if (osiris_host_process_signal_group(resource, signal_number) != 0 && errno != ESRCH) return osiris_host_errno("process_signal");
  return obool(1);
}

static OValue osiris_host_process_release(OValue value) {
  value = osiris_host_process_value(value);
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_PROCESS, "process_release");
  if (!resource) return onull();
  if (!resource->waited) {
    pid_t result = waitpid(resource->pid, &resource->wait_status, WNOHANG);
    if (result == resource->pid) resource->waited = 1;
    else if (result == 0) return oerrorvalue("busy", "process_release requires a waited process");
    else if (result < 0 && errno != ECHILD) return osiris_host_errno("process_release.waitpid");
  }
  osiris_host_resource_close(resource); return obool(1);
}

static OValue osiris_host_process_stream(OValue value, int which) {
  value = osiris_host_process_value(value);
  OHostResource *process = osiris_host_resource_get(value, OSIRIS_HOST_PROCESS, "process_stream");
  if (!process) return onull();
  OHostResource *stream = osiris_host_resource_new(OSIRIS_HOST_STREAM);
  if (!stream) return ofail("out of memory");
  stream->fd = which == 0 ? process->input_fd : (which == 1 ? process->output_fd : process->error_fd);
  if (stream->fd < 0) { osiris_host_resource_close(stream); return ofail("process stream is closed"); }
  if (which == 0) process->input_fd = -1; else if (which == 1) process->output_fd = -1; else process->error_fd = -1;
  return osiris_host_resource_value(stream);
}

static int osiris_host_watch_add(OHostWatchEntry **entries, size_t *count, size_t *capacity, const char *path, const struct stat *info) {
  if (*count >= 65536) return 0;
  if (*count == *capacity) { size_t next = *capacity ? *capacity * 2 : 64; OHostWatchEntry *grown = (OHostWatchEntry *)realloc(*entries, next * sizeof(**entries)); if (!grown) return 0; *entries = grown; *capacity = next; }
  size_t length = strlen(path); (*entries)[*count].path = (char *)malloc(length + 1); if (!(*entries)[*count].path) return 0; memcpy((*entries)[*count].path, path, length + 1); (*entries)[*count].size = info->st_size; (*entries)[*count].modified = info->st_mtime; (*entries)[*count].mode = info->st_mode; (*count)++; return 1;
}

static int osiris_host_watch_collect(const char *root, int recursive, OHostWatchEntry **entries, size_t *count, size_t *capacity) {
  struct stat info; if (lstat(root, &info) != 0) return 0;
  if (!S_ISDIR(info.st_mode)) return osiris_host_watch_add(entries, count, capacity, root, &info);
  DIR *directory = opendir(root); if (!directory) return 0;
  struct dirent *item;
  while ((item = readdir(directory)) != NULL) {
    if (!strcmp(item->d_name, ".") || !strcmp(item->d_name, "..")) continue;
    size_t root_length = strlen(root), item_length = strlen(item->d_name); if (root_length > SIZE_MAX - item_length - 2) { closedir(directory); return 0; }
    char *path = (char *)malloc(root_length + item_length + 2); if (!path) { closedir(directory); return 0; }
    snprintf(path, root_length + item_length + 2, "%s/%s", root, item->d_name);
    struct stat child; int ok = lstat(path, &child) == 0;
    if (ok && S_ISDIR(child.st_mode) && recursive) ok = osiris_host_watch_collect(path, recursive, entries, count, capacity);
    else if (ok && !S_ISDIR(child.st_mode)) ok = osiris_host_watch_add(entries, count, capacity, path, &child);
    free(path); if (!ok) { closedir(directory); return 0; }
  }
  closedir(directory); return 1;
}

static void osiris_host_watch_free_entries(OHostWatchEntry *entries, size_t count) { if (!entries) return; for (size_t i = 0; i < count; i++) free(entries[i].path); free(entries); }
static int osiris_host_watch_find(const OHostWatchEntry *entries, size_t count, const char *path) { for (size_t i = 0; i < count; i++) if (!strcmp(entries[i].path, path)) return (int)i; return -1; }

static int osiris_host_watch_scan(OHostResource *resource) {
  OHostWatchEntry *current = NULL; size_t current_count = 0, current_capacity = 0;
  if (!osiris_host_watch_collect(resource->path, resource->recursive, &current, &current_count, &current_capacity)) { osiris_host_watch_free_entries(current, current_count); return 0; }
  size_t event_capacity = 0, event_length = 0; char *events = NULL;
  for (size_t i = 0; i < current_count; i++) { int old = osiris_host_watch_find(resource->watch_entries, resource->watch_count, current[i].path); const char *kind = old < 0 ? "create" : ((resource->watch_entries[old].size != current[i].size || resource->watch_entries[old].modified != current[i].modified || resource->watch_entries[old].mode != current[i].mode) ? "modify" : NULL); if (!kind) continue; char line[4096]; int line_length = snprintf(line, sizeof(line), "{\"kind\":\"%s\",\"path\":\"%s\"}\n", kind, current[i].path); if (line_length < 0 || (size_t)line_length >= sizeof(line) || (size_t)line_length > SIZE_MAX - event_length - 1 || event_length + (size_t)line_length > OSIRIS_HOST_MAX_IO) { free(events); osiris_host_watch_free_entries(current, current_count); return 0; } if (event_length + (size_t)line_length > event_capacity) { size_t next = event_capacity ? event_capacity * 2 : 4096; while (next < event_length + (size_t)line_length) next *= 2; char *grown = (char *)realloc(events, next); if (!grown) { free(events); osiris_host_watch_free_entries(current, current_count); return 0; } events = grown; event_capacity = next; } memcpy(events + event_length, line, (size_t)line_length); event_length += (size_t)line_length; }
  for (size_t i = 0; i < resource->watch_count; i++) { if (osiris_host_watch_find(current, current_count, resource->watch_entries[i].path) >= 0) continue; char line[4096]; int line_length = snprintf(line, sizeof(line), "{\"kind\":\"remove\",\"path\":\"%s\"}\n", resource->watch_entries[i].path); if (line_length < 0 || (size_t)line_length >= sizeof(line) || event_length + (size_t)line_length > OSIRIS_HOST_MAX_IO) { free(events); osiris_host_watch_free_entries(current, current_count); return 0; } if (event_length + (size_t)line_length > event_capacity) { size_t next = event_capacity ? event_capacity * 2 : 4096; while (next < event_length + (size_t)line_length) next *= 2; char *grown = (char *)realloc(events, next); if (!grown) { free(events); osiris_host_watch_free_entries(current, current_count); return 0; } events = grown; event_capacity = next; } memcpy(events + event_length, line, (size_t)line_length); event_length += (size_t)line_length; }
  osiris_host_watch_free_entries(resource->watch_entries, resource->watch_count); resource->watch_entries = current; resource->watch_count = current_count; if (event_length) { free(resource->watch_pending); resource->watch_pending = events; resource->watch_pending_length = event_length; resource->watch_pending_offset = 0; } else free(events); return 1;
}

static OValue osiris_host_watch(OValue path_value, OValue recursive_value) {
  const char *path = NULL; size_t length = 0; if (!osiris_host_string(path_value, &path, &length) || length == 0 || length > OSIRIS_HOST_MAX_IO) return ofail("watch expects a non-empty path"); int recursive = 1; if (recursive_value.kind != O_NULL) { if (recursive_value.kind != O_BOOL) return ofail("watch recursive argument must be bool"); recursive = recursive_value.as.b; }
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_WATCH); if (!resource) return ofail("out of memory"); resource->path = (char *)malloc(length + 1); if (!resource->path) { osiris_host_resource_close(resource); return ofail("out of memory"); } memcpy(resource->path, path, length); resource->path[length] = 0; resource->recursive = recursive; size_t capacity = 0; if (!osiris_host_watch_collect(resource->path, recursive, &resource->watch_entries, &resource->watch_count, &capacity)) { osiris_host_resource_close(resource); return osiris_host_errno("watch"); } return osiris_host_resource_value(resource);
}

static OValue osiris_host_watch_read(OValue value, OValue size_value, OValue timeout_value) {
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_WATCH, "watch_read"); if (!resource) return onull(); size_t size = size_value.kind == O_UINT && size_value.as.u <= OSIRIS_HOST_MAX_IO ? (size_t)size_value.as.u : 0; if (!size) return ofail("watch_read size must be a bounded unsigned integer"); int timeout = osiris_host_effective_timeout(resource, timeout_value); if (timeout == -2) return osiris_host_errno("watch_read.timeout"); int64_t end = timeout < 0 ? -1 : osiris_host_now_ms() + timeout;
  while (resource->watch_pending_offset >= resource->watch_pending_length) { if (!osiris_host_watch_scan(resource)) return osiris_host_errno("watch_read"); if (resource->watch_pending_length == 0) { if (end >= 0 && osiris_host_now_ms() >= end) return osiris_host_errno("watch_read.timeout"); struct timespec pause_for = {.tv_sec=0, .tv_nsec=50000000L}; nanosleep(&pause_for, NULL); } }
  size_t available = resource->watch_pending_length - resource->watch_pending_offset; if (available > size) available = size; char *copy = (char *)malloc(available + 1); if (!copy) return ofail("out of memory"); memcpy(copy, resource->watch_pending + resource->watch_pending_offset, available); copy[available] = 0; if (!otrack(copy)) { free(copy); return ofail("out of memory"); } OValue result = ostringview(copy, available); resource->watch_pending_offset += available; if (resource->watch_pending_offset == resource->watch_pending_length) { free(resource->watch_pending); resource->watch_pending = NULL; resource->watch_pending_offset = resource->watch_pending_length = 0; } return result;
}

static OValue osiris_host_file_open(OValue path, OValue flags, OValue mode) {
  const char *data = NULL; size_t length = 0;
  if (!osiris_host_string(path, &data, &length) || length == 0) return ofail("open expects a non-empty NUL-free path");
  int oflags = flags.kind == O_INT ? (int)flags.as.i : (flags.kind == O_UINT && flags.as.u <= INT_MAX ? (int)flags.as.u : -1);
  int omode = mode.kind == O_INT ? (int)mode.as.i : (mode.kind == O_UINT && mode.as.u <= INT_MAX ? (int)mode.as.u : 0);
  if (oflags < 0 || omode < 0) return ofail("open flags and mode must be integers");
  int fd = open(data, oflags | O_CLOEXEC, (mode_t)omode);
  if (fd < 0) return osiris_host_errno("open");
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_FILE);
  if (!resource) { osiris_host_close_fd(&fd); return ofail("out of memory"); }
  resource->fd = fd; return osiris_host_resource_value(resource);
}

static OValue osiris_host_stat_info(const char *name, const struct stat *info) {
  if (!name || !info) return ofail("stat expects a path");
  const char *slash = strrchr(name, '/'); const char *base = slash ? slash + 1 : name;
  char modified[64]; struct tm *parts = gmtime(&info->st_mtime);
  if (!parts || strftime(modified, sizeof(modified), "%Y-%m-%dT%H:%M:%SZ", parts) == 0) strcpy(modified, "");
  OValue values[5] = {ostring(base), oint((int64_t)info->st_size), ouint((uint64_t)info->st_mode), obool(S_ISDIR(info->st_mode)), ostring(modified)};
  const char *keys[5] = {"name", "size", "mode", "dir", "mod_time"};
  return omap(keys, values, 5);
}

static OValue osiris_host_stat(OValue path, int follow) {
  const char *data = NULL; size_t length = 0;
  if (!osiris_host_string(path, &data, &length) || length == 0) return ofail("stat expects a non-empty path");
  struct stat info; int result = follow ? stat(data, &info) : lstat(data, &info);
  if (result != 0) return osiris_host_errno(follow ? "stat" : "lstat");
  return osiris_host_stat_info(data, &info);
}

static OValue osiris_host_read_dir(OValue path) {
  const char *data = NULL; size_t length = 0;
  if (!osiris_host_string(path, &data, &length) || length == 0) return ofail("read_dir expects a non-empty path");
  DIR *directory = opendir(data); if (!directory) return osiris_host_errno("read_dir");
  size_t capacity = 16, count = 0; char **names = (char **)malloc(capacity * sizeof(char *));
  if (!names) { closedir(directory); return ofail("out of memory"); }
  struct dirent *entry;
  while ((entry = readdir(directory)) != NULL) {
    if (!strcmp(entry->d_name, ".") || !strcmp(entry->d_name, "..")) continue;
    if (count == capacity) { capacity *= 2; char **grown = (char **)realloc(names, capacity * sizeof(char *)); if (!grown) { for (size_t i=0;i<count;i++) free(names[i]); free(names); closedir(directory); return ofail("out of memory"); } names = grown; }
    names[count] = strdup(entry->d_name); if (!names[count]) { for (size_t i=0;i<count;i++) free(names[i]); free(names); closedir(directory); return ofail("out of memory"); } count++;
  }
  closedir(directory);
  for (size_t i=1;i<count;i++) { char *value=names[i]; size_t j=i; while (j>0 && strcmp(names[j-1],value)>0) { names[j]=names[j-1]; j--; } names[j]=value; }
  OValue *items = count ? (OValue *)malloc(count * sizeof(OValue)) : NULL; if (count && !items) { for (size_t i=0;i<count;i++) free(names[i]); free(names); return ofail("out of memory"); }
  for (size_t i=0;i<count;i++) { items[i] = osiris_host_owned_string(names[i], strlen(names[i])); free(names[i]); } free(names); OValue output = olist(items, count); free(items); return output;
}

static int osiris_host_mkdir_all_path(const char *path, mode_t mode) {
  size_t length = strlen(path);
  if (length == 0 || length > OSIRIS_HOST_MAX_IO) { errno = EINVAL; return -1; }
  char *copy = strdup(path);
  if (!copy) { errno = ENOMEM; return -1; }
  for (char *cursor = copy; *cursor; cursor++) {
    if (*cursor != '/' || cursor == copy) continue;
    *cursor = 0;
    if (*copy && mkdir(copy, mode) != 0 && errno != EEXIST) { free(copy); return -1; }
    *cursor = '/';
  }
  if (mkdir(copy, mode) != 0 && errno != EEXIST) { free(copy); return -1; }
  struct stat info;
  int result = stat(copy, &info);
  if (result != 0 || !S_ISDIR(info.st_mode)) { if (result == 0) errno = ENOTDIR; free(copy); return -1; }
  free(copy);
  return 0;
}

static int osiris_host_remove_all_path(const char *path) {
  struct stat info;
  if (lstat(path, &info) != 0) {
    if (errno == ENOENT) return 0;
    return -1;
  }
  if (!S_ISDIR(info.st_mode)) return unlink(path);
  DIR *directory = opendir(path);
  if (!directory) return -1;
  struct dirent *entry;
  while ((entry = readdir(directory)) != NULL) {
    if (!strcmp(entry->d_name, ".") || !strcmp(entry->d_name, "..")) continue;
    size_t path_length = strlen(path), name_length = strlen(entry->d_name);
    if (path_length > SIZE_MAX - name_length - 2) { closedir(directory); errno = ENAMETOOLONG; return -1; }
    char *child = (char *)malloc(path_length + name_length + 2);
    if (!child) { closedir(directory); errno = ENOMEM; return -1; }
    snprintf(child, path_length + name_length + 2, "%s/%s", path, entry->d_name);
    int result = osiris_host_remove_all_path(child);
    int saved = errno;
    free(child);
    if (result != 0) { closedir(directory); errno = saved; return -1; }
  }
  closedir(directory);
  return rmdir(path);
}

static OValue osiris_host_path_string(OValue value, const char *operation, const char **data, size_t *length) {
  if (!osiris_host_string(value, data, length) || *length == 0 || *length > OSIRIS_HOST_MAX_IO) return ofail(operation);
  return onull();
}

static OValue osiris_host_mkdir(OValue path, OValue mode, int all) {
  const char *data = NULL; size_t length = 0;
  OValue validation = osiris_host_path_string(path, all ? "mkdir_all expects a non-empty bounded path" : "mkdir expects a non-empty bounded path", &data, &length);
  if (validation.kind == O_ERROR) return validation;
  int64_t mode_value = mode.kind == O_INT ? mode.as.i : (mode.kind == O_UINT && mode.as.u <= UINT32_MAX ? (int64_t)mode.as.u : -1);
  if (mode_value < 0) return ofail("mkdir expects an integer mode");
  int result = all ? osiris_host_mkdir_all_path(data, (mode_t)mode_value) : mkdir(data, (mode_t)mode_value);
  if (result != 0) return osiris_host_errno(all ? "mkdir_all" : "mkdir");
  return obool(1);
}

static OValue osiris_host_remove(OValue path, int all) {
  const char *data = NULL; size_t length = 0;
  OValue validation = osiris_host_path_string(path, all ? "remove_all expects a non-empty bounded path" : "remove expects a non-empty bounded path", &data, &length);
  if (validation.kind == O_ERROR) return validation;
  int result = all ? osiris_host_remove_all_path(data) : remove(data);
  if (result != 0) return osiris_host_errno(all ? "remove_all" : "remove");
  return obool(1);
}

static OValue osiris_host_truncate(OValue path, OValue size) {
  const char *data = NULL; size_t length = 0;
  OValue validation = osiris_host_path_string(path, "truncate expects a non-empty bounded path", &data, &length);
  if (validation.kind == O_ERROR) return validation;
  uint64_t size_value = size.kind == O_UINT ? size.as.u : (size.kind == O_INT && size.as.i >= 0 ? (uint64_t)size.as.i : UINT64_MAX);
  if (size_value == UINT64_MAX || size_value > (uint64_t)INT64_MAX) return ofail("truncate expects a non-negative bounded size");
  if (truncate(data, (off_t)size_value) != 0) return osiris_host_errno("truncate");
  return obool(1);
}

static OValue osiris_host_rename(OValue old_path, OValue new_path) {
  const char *old_data = NULL, *new_data = NULL; size_t old_length = 0, new_length = 0;
  OValue old_validation = osiris_host_path_string(old_path, "rename expects a non-empty old path", &old_data, &old_length);
  if (old_validation.kind == O_ERROR) return old_validation;
  OValue new_validation = osiris_host_path_string(new_path, "rename expects a non-empty new path", &new_data, &new_length);
  if (new_validation.kind == O_ERROR) return new_validation;
  if (rename(old_data, new_data) != 0) return osiris_host_errno("rename");
  return obool(1);
}

static OValue osiris_host_readlink(OValue path) {
  const char *data = NULL; size_t length = 0;
  OValue validation = osiris_host_path_string(path, "readlink expects a non-empty bounded path", &data, &length);
  if (validation.kind == O_ERROR) return validation;
  size_t capacity = 256;
  while (capacity <= OSIRIS_HOST_MAX_IO) {
    char *target = (char *)malloc(capacity);
    if (!target) return ofail("out of memory");
    ssize_t result = readlink(data, target, capacity);
    if (result < 0) { free(target); return osiris_host_errno("readlink"); }
    if ((size_t)result < capacity) {
      OValue output = osiris_host_owned_string(target, (size_t)result);
      free(target);
      return output;
    }
    free(target);
    if (capacity > OSIRIS_HOST_MAX_IO / 2) break;
    capacity *= 2;
  }
  return ofail("readlink target exceeds the native bound");
}

static OValue osiris_host_chmod(OValue path, OValue mode) {
  const char *data = NULL; size_t length = 0;
  OValue validation = osiris_host_path_string(path, "chmod expects a non-empty bounded path", &data, &length);
  if (validation.kind == O_ERROR) return validation;
  uint64_t mode_value = mode.kind == O_UINT ? mode.as.u : (mode.kind == O_INT && mode.as.i >= 0 ? (uint64_t)mode.as.i : UINT64_MAX);
  if (mode_value == UINT64_MAX || mode_value > UINT32_MAX) return ofail("chmod expects a bounded integer mode");
  if (chmod(data, (mode_t)mode_value) != 0) return osiris_host_errno("chmod");
  return obool(1);
}

static OValue osiris_host_link(OValue old_path, OValue new_path, int symbolic) {
  const char *old_data = NULL, *new_data = NULL; size_t old_length = 0, new_length = 0;
  OValue old_validation = osiris_host_path_string(old_path, symbolic ? "symlink expects a non-empty old path" : "link expects a non-empty old path", &old_data, &old_length);
  if (old_validation.kind == O_ERROR) return old_validation;
  OValue new_validation = osiris_host_path_string(new_path, symbolic ? "symlink expects a non-empty new path" : "link expects a non-empty new path", &new_data, &new_length);
  if (new_validation.kind == O_ERROR) return new_validation;
  int result = symbolic ? symlink(old_data, new_data) : link(old_data, new_data);
  if (result != 0) return osiris_host_errno(symbolic ? "symlink" : "link");
  return obool(1);
}

static OHostResource *osiris_host_io_resource(OValue value, const char *operation);

static OValue osiris_host_file_write_string(OValue value, OValue string_value, OValue timeout) {
  OHostResource *resource = osiris_host_io_resource(value, "write_string");
  if (!resource || string_value.kind != O_STRING || string_value.as.s.len > OSIRIS_HOST_MAX_IO) return ofail("write_string expects a bounded string handle payload");
  int timeout_ms = osiris_host_effective_timeout(resource, timeout);
  if (timeout_ms == -2) return osiris_host_errno("write_string.timeout");
  if (osiris_host_wait_fd(resource->fd, POLLOUT, timeout_ms) <= 0) return osiris_host_errno("write_string.poll");
  ssize_t result = write(resource->fd, string_value.as.s.data, string_value.as.s.len);
  if (result < 0) return osiris_host_errno("write_string");
  return ouint((uint64_t)result);
}

static OValue osiris_host_file_close(OValue value) {
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_FILE, "close");
  if (!resource) return onull();
  osiris_host_resource_close(resource); return obool(1);
}

static OHostResource *osiris_host_io_resource(OValue value, const char *operation) {
  if (value.kind != O_RESOURCE || !value.as.resource) { ofail("host stream or file handle expected"); return NULL; }
  OHostResource *resource = (OHostResource *)value.as.resource;
  if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed || (resource->kind != OSIRIS_HOST_FILE && resource->kind != OSIRIS_HOST_STREAM)) {
    ofail("host stream or file handle is closed or invalid"); return NULL;
  }
  (void)operation; return resource;
}

static OValue osiris_host_file_io(OValue value, OValue data_or_size, OValue offset, OValue timeout, int writing, int positioned) {
  OHostResource *resource = osiris_host_io_resource(value, writing ? "write" : "read");
  if (!resource) return onull();
  int timeout_ms = osiris_host_effective_timeout(resource, timeout);
  if (timeout_ms == -2) return osiris_host_errno("file_io.timeout");
  if (osiris_host_wait_fd(resource->fd, writing ? POLLOUT : POLLIN, timeout_ms) <= 0) return osiris_host_errno("file_io.poll");
  if (writing) {
    unsigned char *bytes = NULL; size_t length = 0;
    if (data_or_size.kind != O_LIST || data_or_size.as.list.len > OSIRIS_HOST_MAX_IO) return ofail("write expects a bounded byte list");
    length = data_or_size.as.list.len; bytes = (unsigned char *)malloc(length ? length : 1);
    if (!bytes && length) return ofail("out of memory");
    for (size_t i = 0; i < length; i++) { OValue item = data_or_size.as.list.items[i]; if (item.kind != O_UINT || item.as.u > 255) { free(bytes); return ofail("write expects u8 values"); } bytes[i] = (unsigned char)item.as.u; }
    ssize_t result = positioned ? pwrite(resource->fd, bytes, length, (off_t)(offset.kind == O_INT ? offset.as.i : offset.as.u)) : write(resource->fd, bytes, length);
    free(bytes); if (result < 0) return osiris_host_errno("write"); return ouint((uint64_t)result);
  }
  size_t length = data_or_size.kind == O_UINT && data_or_size.as.u <= OSIRIS_HOST_MAX_IO ? (size_t)data_or_size.as.u : (data_or_size.kind == O_INT && data_or_size.as.i >= 0 && data_or_size.as.i <= OSIRIS_HOST_MAX_IO ? (size_t)data_or_size.as.i : 0);
  if (!length) return olist(NULL, 0);
  unsigned char *bytes = (unsigned char *)malloc(length); if (!bytes) return ofail("out of memory");
  ssize_t result = positioned ? pread(resource->fd, bytes, length, (off_t)(offset.kind == O_INT ? offset.as.i : offset.as.u)) : read(resource->fd, bytes, length);
  if (result < 0) { free(bytes); return osiris_host_errno("read"); }
  OValue output = osiris_host_bytes(bytes, (size_t)result); free(bytes); return output;
}

static OValue osiris_host_file_seek(OValue value, OValue offset, OValue whence) {
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_FILE, "seek_file"); if (!resource) return onull();
  int64_t position = offset.kind == O_INT ? offset.as.i : (offset.kind == O_UINT && offset.as.u <= INT64_MAX ? (int64_t)offset.as.u : INT64_MIN);
  int origin = whence.kind == O_INT ? (int)whence.as.i : (whence.kind == O_UINT && whence.as.u <= INT_MAX ? (int)whence.as.u : -1);
  if (position == INT64_MIN || origin < SEEK_SET || origin > SEEK_END) return ofail("seek_file expects a valid offset and origin");
  off_t result = lseek(resource->fd, (off_t)position, origin); if (result < 0) return osiris_host_errno("seek_file"); return oint((int64_t)result);
}

static OValue osiris_host_file_sync(OValue value) {
  OHostResource *resource = osiris_host_resource_get(value, OSIRIS_HOST_FILE, "sync"); if (!resource) return onull();
  if (fsync(resource->fd) != 0) return osiris_host_errno("sync"); return onull();
}

static int osiris_host_root_relative(const char *path, size_t length) {
  if (!path || length == 0 || path[0] == '/') return 0;
  size_t start = 0;
  for (size_t i = 0; i <= length; i++) {
    if (i != length && path[i] != '/') continue;
    size_t component_length = i - start;
    if (component_length == 2 && path[start] == '.' && path[start + 1] == '.') return 0;
    start = i + 1;
  }
  return 1;
}

/* Resolve a rooted path without ever allowing a symlink component to select
 * a directory outside root_fd.  The resolver deliberately rejects absolute
 * and .. symlink targets.  That is the portable POSIX contract: a link may
 * point to another relative entry below the root, but no path operation can
 * escape through link expansion.  O_NOFOLLOW on the final open closes the
 * check/open race conservatively by failing rather than following a changed
 * link. */
static int osiris_host_root_open_followed(int root_fd, const char *path, int flags, mode_t mode, const char *operation, int *fd_out) {
  if (!path || !osiris_host_root_relative(path, strlen(path))) { ofail("root path must be a bounded relative path without .."); return 0; }
  char *pending = strdup(path);
  if (!pending) { ofail("out of memory"); return 0; }
  int current = dup(root_fd);
  if (current < 0) { int saved = errno; free(pending); errno = saved; osiris_host_errno(operation); return 0; }
  unsigned symlink_depth = 0;
  for (;;) {
    while (*pending == '/') memmove(pending, pending + 1, strlen(pending));
    if (*pending == 0) { *fd_out = current; free(pending); return 1; }
    char *component = pending;
    char *separator = strchr(component, '/');
    char *rest = NULL;
    if (separator) { *separator = 0; rest = separator + 1; }
    else rest = component + strlen(component);
    if (component[0] == 0 || strcmp(component, ".") == 0) {
      if (*rest) memmove(pending, rest, strlen(rest) + 1);
      else *pending = 0;
      continue;
    }
    if (strcmp(component, "..") == 0) { close(current); free(pending); ofail("root path contains an invalid directory component"); return 0; }
    int final = *rest == 0;
    struct stat info;
    int have_info = fstatat(current, component, &info, AT_SYMLINK_NOFOLLOW) == 0;
    if (!have_info) {
      if (!final || !(flags & O_CREAT)) { int saved = errno; close(current); free(pending); errno = saved; osiris_host_errno(operation); return 0; }
    }
    if (!final || (flags & O_NOFOLLOW) == 0) {
      if (have_info && S_ISLNK(info.st_mode)) {
        if (++symlink_depth > 40) { close(current); free(pending); errno = ELOOP; osiris_host_errno(operation); return 0; }
        size_t capacity = 256;
        char *target = NULL;
        ssize_t target_length = -1;
        while (capacity <= OSIRIS_HOST_MAX_IO) {
          target = (char *)malloc(capacity);
          if (!target) { close(current); free(pending); ofail("out of memory"); return 0; }
          target_length = readlinkat(current, component, target, capacity);
          if (target_length < 0) { int saved = errno; free(target); close(current); free(pending); errno = saved; osiris_host_errno(operation); return 0; }
          if ((size_t)target_length < capacity) break;
          free(target); target = NULL;
          if (capacity > OSIRIS_HOST_MAX_IO / 2) break;
          capacity *= 2;
        }
        if (!target || target_length < 0 || (size_t)target_length >= capacity || !osiris_host_root_relative(target, (size_t)target_length)) {
          free(target); close(current); free(pending); ofail("root symlink target is absolute, escaping, or too large"); return 0;
        }
        size_t rest_length = strlen(rest);
        size_t rest_extra = rest_length ? rest_length + 1 : 0;
        if (rest_length > OSIRIS_HOST_MAX_IO || rest_extra > OSIRIS_HOST_MAX_IO || (size_t)target_length > OSIRIS_HOST_MAX_IO - rest_extra) { free(target); close(current); free(pending); ofail("root symlink expansion exceeds the native bound"); return 0; }
        char *expanded = (char *)malloc((size_t)target_length + rest_extra + 1);
        if (!expanded) { free(target); close(current); free(pending); ofail("out of memory"); return 0; }
        memcpy(expanded, target, (size_t)target_length);
        if (rest_length) { expanded[target_length] = '/'; memcpy(expanded + target_length + 1, rest, rest_length); expanded[target_length + rest_length + 1] = 0; }
        else expanded[target_length] = 0;
        free(target); free(pending); pending = expanded; continue;
      }
    }
    if (!final) {
      int next = openat(current, component, O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
      if (next < 0) { int saved = errno; close(current); free(pending); errno = saved; osiris_host_errno(operation); return 0; }
      close(current); current = next;
      char *next_pending = strdup(rest);
      if (!next_pending) { close(current); free(pending); ofail("out of memory"); return 0; }
      free(pending); pending = next_pending;
      continue;
    }
    int opened = openat(current, component, flags | O_CLOEXEC | O_NOFOLLOW, mode);
    if (opened < 0) { int saved = errno; close(current); free(pending); errno = saved; osiris_host_errno(operation); return 0; }
    close(current); free(pending); *fd_out = opened; return 1;
  }
}

static int osiris_host_root_name(OValue root_value, OValue path_value, const char *operation, OHostResource **root_out, const char **path_out) {
  OHostResource *root = osiris_host_resource_get(root_value, OSIRIS_HOST_ROOT, operation);
  if (!root) return 0;
  const char *path = NULL; size_t length = 0;
  if (!osiris_host_string(path_value, &path, &length) || length > OSIRIS_HOST_MAX_IO || !osiris_host_root_relative(path, length)) {
    ofail("root path must be a bounded relative path without .."); return 0;
  }
  if (root_out) *root_out = root;
  if (path_out) *path_out = path;
  return 1;
}

static int osiris_host_root_openat(OValue root_value, OValue path_value, int flags, mode_t mode, const char *operation, int *fd_out) {
  OHostResource *root = NULL; const char *path = NULL;
  if (!osiris_host_root_name(root_value, path_value, operation, &root, &path)) return 0;
  return osiris_host_root_open_followed(root->fd, path, flags, mode, operation, fd_out);
}

static OValue osiris_host_root_open(OValue path) {
  const char *data = NULL; size_t length = 0;
  if (!osiris_host_string(path, &data, &length) || length == 0) return ofail("open_root expects a non-empty NUL-free path");
  int fd = open(data, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
  if (fd < 0) return osiris_host_errno("open_root");
  OHostResource *resource = osiris_host_resource_new(OSIRIS_HOST_ROOT);
  if (!resource) { osiris_host_close_fd(&fd); return ofail("out of memory"); }
  resource->fd = fd; return osiris_host_resource_value(resource);
}

static OValue osiris_host_root_open_file(OValue value, OValue path, OValue flags, OValue mode) {
  OHostResource *root = osiris_host_resource_get(value, OSIRIS_HOST_ROOT, "root_open");
  if (!root) return onull();
  const char *data = NULL; size_t length = 0;
  if (!osiris_host_string(path, &data, &length) || !osiris_host_root_relative(data, length)) return ofail("root_open requires a relative path without ..");
  int oflags = flags.kind == O_INT ? (int)flags.as.i : (flags.kind == O_UINT && flags.as.u <= INT_MAX ? (int)flags.as.u : -1);
  int omode = mode.kind == O_INT ? (int)mode.as.i : (mode.kind == O_UINT && mode.as.u <= INT_MAX ? (int)mode.as.u : 0);
  if (oflags < 0 || omode < 0) return ofail("root_open flags and mode must be integers");
  int fd = -1;
  if (!osiris_host_root_open_followed(root->fd, data, oflags, (mode_t)omode, "root_open", &fd)) return onull();
  OHostResource *file = osiris_host_resource_new(OSIRIS_HOST_FILE);
  if (!file) { osiris_host_close_fd(&fd); return ofail("out of memory"); }
  file->fd = fd; return osiris_host_resource_value(file);
}

static OValue osiris_host_root_create(OValue root, OValue path) {
  int fd = -1;
  if (!osiris_host_root_openat(root, path, O_RDWR | O_CREAT | O_TRUNC, 0666, "root_create", &fd)) return onull();
  OHostResource *file = osiris_host_resource_new(OSIRIS_HOST_FILE);
  if (!file) { osiris_host_close_fd(&fd); return ofail("out of memory"); }
  file->fd = fd; return osiris_host_resource_value(file);
}

static OValue osiris_host_root_read_file(OValue root, OValue path) {
  int fd = -1;
  if (!osiris_host_root_openat(root, path, O_RDONLY, 0, "root_read_file", &fd)) return onull();
  unsigned char *bytes = (unsigned char *)malloc(OSIRIS_HOST_MAX_IO + 1);
  if (!bytes) { osiris_host_close_fd(&fd); return ofail("out of memory"); }
  size_t total = 0;
  while (total <= OSIRIS_HOST_MAX_IO) {
    ssize_t count = read(fd, bytes + total, OSIRIS_HOST_MAX_IO + 1 - total);
    if (count == 0) break;
    if (count < 0 && errno == EINTR) continue;
    if (count < 0) { osiris_host_close_fd(&fd); free(bytes); return osiris_host_errno("root_read_file"); }
    total += (size_t)count;
    if (total > OSIRIS_HOST_MAX_IO) { osiris_host_close_fd(&fd); free(bytes); return ofail("root_read_file exceeds the native bound"); }
  }
  osiris_host_close_fd(&fd);
  OValue result = osiris_host_bytes(bytes, total); free(bytes); return result;
}

static OValue osiris_host_root_write_file(OValue root, OValue path, OValue data_value, OValue mode_value) {
  int mode = mode_value.kind == O_INT ? (int)mode_value.as.i : (mode_value.kind == O_UINT && mode_value.as.u <= INT_MAX ? (int)mode_value.as.u : -1);
  if (mode < 0) return ofail("root_write_file mode must be an integer");
  unsigned char *data = NULL; size_t length = 0;
  if (!osiris_host_value_bytes(data_value, &data, &length)) return ofail("root_write_file expects bounded string or u8 list data");
  int fd = -1;
  if (!osiris_host_root_openat(root, path, O_WRONLY | O_CREAT | O_TRUNC, (mode_t)mode, "root_write_file", &fd)) { free(data); return onull(); }
  size_t written = 0;
  while (written < length) {
    ssize_t count = write(fd, data + written, length - written);
    if (count < 0 && errno == EINTR) continue;
    if (count < 0) { osiris_host_close_fd(&fd); free(data); return osiris_host_errno("root_write_file"); }
    if (count == 0) { osiris_host_close_fd(&fd); free(data); return ofail("root_write_file made no progress"); }
    written += (size_t)count;
  }
  if (close(fd) != 0) { free(data); return osiris_host_errno("root_write_file.close"); }
  free(data); return obool(1);
}

static int osiris_host_root_parent_fd(OValue root_value, OValue path_value, const char *operation, int *parent_fd_out, char **leaf_out);

static OValue osiris_host_root_readlink(OValue root_value, OValue path_value) {
  if (!osiris_host_root_name(root_value, path_value, "root_readlink", NULL, NULL)) return onull();
  int parent_fd = -1; char *leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, path_value, "root_readlink", &parent_fd, &leaf)) return onull();
  size_t capacity = 256;
  while (capacity <= OSIRIS_HOST_MAX_IO) {
    char *target = (char *)malloc(capacity);
    if (!target) { close(parent_fd); free(leaf); return ofail("out of memory"); }
    ssize_t result = readlinkat(parent_fd, leaf, target, capacity);
    if (result < 0) { int saved = errno; free(target); close(parent_fd); free(leaf); errno = saved; return osiris_host_errno("root_readlink"); }
    if ((size_t)result < capacity) {
      OValue output = osiris_host_owned_string(target, (size_t)result);
      free(target); close(parent_fd); free(leaf);
      return output;
    }
    free(target);
    if (capacity > OSIRIS_HOST_MAX_IO / 2) break;
    capacity *= 2;
  }
  close(parent_fd); free(leaf);
  return ofail("root_readlink target exceeds the native bound");
}

static OValue osiris_host_root_link(OValue root_value, OValue old_value, OValue new_value, int symbolic) {
  const char *old_path = NULL; const char *new_path = NULL;
  if (!osiris_host_root_name(root_value, old_value, symbolic ? "root_symlink" : "root_link", NULL, &old_path) || !osiris_host_root_name(root_value, new_value, symbolic ? "root_symlink" : "root_link", NULL, &new_path)) return onull();
  int new_parent = -1; char *new_leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, new_value, symbolic ? "root_symlink" : "root_link", &new_parent, &new_leaf)) return onull();
  int result = -1;
  if (symbolic) {
    result = symlinkat(old_path, new_parent, new_leaf);
  } else {
    int old_parent = -1; char *old_leaf = NULL;
    if (!osiris_host_root_parent_fd(root_value, old_value, "root_link", &old_parent, &old_leaf)) { close(new_parent); free(new_leaf); return onull(); }
    result = linkat(old_parent, old_leaf, new_parent, new_leaf, 0);
    close(old_parent); free(old_leaf);
  }
  int saved = errno; close(new_parent); free(new_leaf); errno = saved;
  if (result != 0) return osiris_host_errno(symbolic ? "root_symlink" : "root_link");
  return obool(1);
}

static OValue osiris_host_root_stat(OValue root_value, OValue path_value, int follow) {
  OHostResource *root = NULL; const char *path = NULL;
  if (!osiris_host_root_name(root_value, path_value, follow ? "root_stat" : "root_lstat", &root, &path)) return onull();
  struct stat info;
  if (follow) {
    int fd = -1;
    if (!osiris_host_root_open_followed(root->fd, path, O_RDONLY | O_NONBLOCK, 0, "root_stat", &fd)) return onull();
    if (fstat(fd, &info) != 0) { int saved = errno; close(fd); errno = saved; return osiris_host_errno("root_stat"); }
    close(fd);
  } else {
    int parent_fd = -1; char *leaf = NULL;
    if (!osiris_host_root_parent_fd(root_value, path_value, "root_lstat", &parent_fd, &leaf)) return onull();
    int result = fstatat(parent_fd, leaf, &info, AT_SYMLINK_NOFOLLOW);
    int saved = errno; close(parent_fd); free(leaf); errno = saved;
    if (result != 0) return osiris_host_errno("root_lstat");
  }
  return osiris_host_stat_info(path, &info);
}

static int osiris_host_root_open_directory_path(int root_fd, char *path, const char *operation, int *fd_out) {
  int current = dup(root_fd);
  if (current < 0) { osiris_host_errno(operation); return 0; }
  char *save = NULL;
  for (char *component = strtok_r(path, "/", &save); component; component = strtok_r(NULL, "/", &save)) {
    if (component[0] == 0 || strcmp(component, ".") == 0 || strcmp(component, "..") == 0) {
      close(current); ofail("root path contains an invalid directory component"); return 0;
    }
    int next = openat(current, component, O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
    if (next < 0) { int saved = errno; close(current); errno = saved; osiris_host_errno(operation); return 0; }
    close(current); current = next;
  }
  *fd_out = current; return 1;
}

static int osiris_host_root_remove_tree_at(int parent_fd, const char *name, unsigned depth) {
  if (depth > 128) { errno = ELOOP; return 0; }
  struct stat info;
  if (fstatat(parent_fd, name, &info, AT_SYMLINK_NOFOLLOW) != 0) return 0;
  if (!S_ISDIR(info.st_mode)) return unlinkat(parent_fd, name, 0) == 0;
  int child_fd = openat(parent_fd, name, O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
  if (child_fd < 0) return 0;
  DIR *directory = fdopendir(child_fd);
  if (!directory) { int saved = errno; close(child_fd); errno = saved; return 0; }
  struct dirent *entry = NULL;
  int success = 1;
  while ((entry = readdir(directory)) != NULL) {
    if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) continue;
    if (!osiris_host_root_remove_tree_at(child_fd, entry->d_name, depth + 1)) { success = 0; break; }
  }
  int saved = errno;
  closedir(directory);
  errno = saved;
  if (!success) return 0;
  return unlinkat(parent_fd, name, AT_REMOVEDIR) == 0;
}

static int osiris_host_root_parent_fd(OValue root_value, OValue path_value, const char *operation, int *parent_fd_out, char **leaf_out) {
  OHostResource *root = NULL; const char *path = NULL; size_t length = 0;
  if (!osiris_host_root_name(root_value, path_value, operation, &root, &path)) return 0;
  if (!osiris_host_string(path_value, &path, &length) || length == 0) { ofail("root path must name an entry"); return 0; }
  char *copy = strdup(path);
  if (!copy) { ofail("out of memory"); return 0; }
  char *separator = strrchr(copy, '/');
  char *leaf = copy;
  int parent_fd = -1;
  if (separator) {
    *separator = 0; leaf = separator + 1;
    if (leaf[0] == 0 || !osiris_host_root_open_directory_path(root->fd, copy, operation, &parent_fd)) { free(copy); return 0; }
  } else {
    parent_fd = dup(root->fd);
    if (parent_fd < 0) { int saved = errno; free(copy); errno = saved; osiris_host_errno(operation); return 0; }
  }
  char *leaf_copy = strdup(leaf);
  free(copy);
  if (!leaf_copy) { close(parent_fd); ofail("out of memory"); return 0; }
  *parent_fd_out = parent_fd; *leaf_out = leaf_copy; return 1;
}

static OValue osiris_host_root_mkdir_all(OValue root_value, OValue path_value, OValue mode_value) {
  OHostResource *root = NULL; const char *path = NULL; size_t length = 0;
  if (!osiris_host_root_name(root_value, path_value, "root_mkdir_all", &root, &path)) return onull();
  if (!osiris_host_string(path_value, &path, &length) || length == 0) return ofail("root_mkdir_all requires a path");
  int mode = mode_value.kind == O_INT ? (int)mode_value.as.i : (mode_value.kind == O_UINT && mode_value.as.u <= INT_MAX ? (int)mode_value.as.u : -1);
  if (mode < 0) return ofail("root_mkdir_all mode must be an integer");
  char *copy = strdup(path); if (!copy) return ofail("out of memory");
  int current = dup(root->fd); if (current < 0) { int saved = errno; free(copy); errno = saved; return osiris_host_errno("root_mkdir_all"); }
  char *save = NULL; int success = 1;
  for (char *component = strtok_r(copy, "/", &save); component; component = strtok_r(NULL, "/", &save)) {
    if (component[0] == 0 || strcmp(component, ".") == 0 || strcmp(component, "..") == 0) { ofail("root path contains an invalid directory component"); success = 0; break; }
    if (mkdirat(current, component, (mode_t)mode) != 0 && errno != EEXIST) { osiris_host_errno("root_mkdir_all"); success = 0; break; }
    int next = openat(current, component, O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
    if (next < 0) { osiris_host_errno("root_mkdir_all"); success = 0; break; }
    close(current); current = next;
  }
  close(current); free(copy); return success ? obool(1) : onull();
}

static OValue osiris_host_root_mkdir(OValue root_value, OValue path_value, OValue mode_value) {
  int mode = mode_value.kind == O_INT ? (int)mode_value.as.i : (mode_value.kind == O_UINT && mode_value.as.u <= INT_MAX ? (int)mode_value.as.u : -1);
  if (mode < 0) return ofail("root_mkdir mode must be an integer");
  int parent_fd = -1; char *leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, path_value, "root_mkdir", &parent_fd, &leaf)) return onull();
  int result = mkdirat(parent_fd, leaf, (mode_t)mode);
  int saved = errno; close(parent_fd); free(leaf); errno = saved;
  if (result != 0) return osiris_host_errno("root_mkdir");
  return obool(1);
}

static OValue osiris_host_root_remove(OValue root_value, OValue path_value) {
  int parent_fd = -1; char *leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, path_value, "root_remove", &parent_fd, &leaf)) return onull();
  if (unlinkat(parent_fd, leaf, 0) == 0) { close(parent_fd); free(leaf); return obool(1); }
  int saved = errno;
  if (saved != EISDIR && saved != EPERM) { close(parent_fd); free(leaf); errno = saved; return osiris_host_errno("root_remove"); }
  if (unlinkat(parent_fd, leaf, AT_REMOVEDIR) != 0) { saved = errno; close(parent_fd); free(leaf); errno = saved; return osiris_host_errno("root_remove"); }
  close(parent_fd); free(leaf);
  return obool(1);
}

static OValue osiris_host_root_remove_all(OValue root_value, OValue path_value) {
  int parent_fd = -1; char *leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, path_value, "root_remove_all", &parent_fd, &leaf)) return onull();
  int result = osiris_host_root_remove_tree_at(parent_fd, leaf, 0);
  int saved = errno; close(parent_fd); free(leaf); errno = saved;
  if (!result) return osiris_host_errno("root_remove_all");
  return obool(1);
}

static OValue osiris_host_root_rename(OValue root_value, OValue old_value, OValue new_value) {
  int old_parent = -1; char *old_leaf = NULL; int new_parent = -1; char *new_leaf = NULL;
  if (!osiris_host_root_parent_fd(root_value, old_value, "root_rename", &old_parent, &old_leaf)) return onull();
  if (!osiris_host_root_parent_fd(root_value, new_value, "root_rename", &new_parent, &new_leaf)) { close(old_parent); free(old_leaf); return onull(); }
  int result = renameat(old_parent, old_leaf, new_parent, new_leaf);
  int saved = errno; close(old_parent); close(new_parent); free(old_leaf); free(new_leaf); errno = saved;
  if (result != 0) return osiris_host_errno("root_rename");
  return obool(1);
}

static OValue osiris_host_root_chmod(OValue root_value, OValue path_value, OValue mode_value) {
  OHostResource *root = NULL; const char *path = NULL;
  if (!osiris_host_root_name(root_value, path_value, "root_chmod", &root, &path)) return onull();
  int mode = mode_value.kind == O_INT ? (int)mode_value.as.i : (mode_value.kind == O_UINT && mode_value.as.u <= INT_MAX ? (int)mode_value.as.u : -1);
  if (mode < 0) return ofail("root_chmod mode must be an integer");
  int fd = -1;
  if (!osiris_host_root_open_followed(root->fd, path, O_RDONLY | O_NONBLOCK, 0, "root_chmod", &fd)) return onull();
  int result = fchmod(fd, (mode_t)mode);
  int saved = errno; close(fd); errno = saved;
  if (result != 0) return osiris_host_errno("root_chmod");
  return obool(1);
}

static int osiris_host_decimal(const char *text, size_t length, size_t *position, size_t digits, int *value) {
  if (!text || !position || !value || *position > length || digits > length - *position) return 0;
  int result = 0;
  for (size_t i = 0; i < digits; i++) {
    unsigned char digit = (unsigned char)text[*position + i];
    if (digit < '0' || digit > '9' || result > (INT_MAX - (digit - '0')) / 10) return 0;
    result = result * 10 + (digit - '0');
  }
  *position += digits; *value = result; return 1;
}

static int64_t osiris_host_days_from_civil(int year, unsigned month, unsigned day) {
  year -= month <= 2;
  int era = (year >= 0 ? year : year - 399) / 400;
  unsigned year_of_era = (unsigned)(year - era * 400);
  unsigned day_of_year = (153 * (month + (month > 2 ? (unsigned)-3 : 9)) + 2) / 5 + day - 1;
  unsigned day_of_era = year_of_era * 365 + year_of_era / 4 - year_of_era / 100 + day_of_year;
  return (int64_t)era * 146097 + (int64_t)day_of_era - 719468;
}

static int osiris_host_parse_timestamp(OValue value, struct timespec *result) {
  const char *text = NULL; size_t length = 0;
  if (!result || !osiris_host_string(value, &text, &length) || length < 20 || text[4] != '-' || text[7] != '-' || text[10] != 'T' || text[13] != ':' || text[16] != ':') return 0;
  size_t position = 0; int year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0;
  if (!osiris_host_decimal(text, length, &position, 4, &year) || position++ >= length || text[position - 1] != '-' || !osiris_host_decimal(text, length, &position, 2, &month) || position++ >= length || text[position - 1] != '-' || !osiris_host_decimal(text, length, &position, 2, &day) || position++ >= length || text[position - 1] != 'T' || !osiris_host_decimal(text, length, &position, 2, &hour) || position++ >= length || text[position - 1] != ':' || !osiris_host_decimal(text, length, &position, 2, &minute) || position++ >= length || text[position - 1] != ':' || !osiris_host_decimal(text, length, &position, 2, &second)) return 0;
  if (month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59) return 0;
  int nanoseconds = 0;
  if (position < length && text[position] == '.') {
    position++;
    size_t start = position, count = 0;
    while (position < length && text[position] >= '0' && text[position] <= '9') {
      if (count < 9) nanoseconds = nanoseconds * 10 + (text[position] - '0');
      count++; position++;
    }
    if (count == 0 || count > 9) return 0;
    while (count++ < 9) nanoseconds *= 10;
    (void)start;
  }
  int offset_minutes = 0;
  if (position == length - 1 && text[position] == 'Z') {
    position++;
  } else {
    if (position >= length || (text[position] != '+' && text[position] != '-') || length - position != 6 || text[position + 3] != ':') return 0;
    int zone_hour = 0, zone_minute = 0; position++;
    if (!osiris_host_decimal(text, length, &position, 2, &zone_hour) || position++ >= length || text[position - 1] != ':' || !osiris_host_decimal(text, length, &position, 2, &zone_minute) || zone_hour > 23 || zone_minute > 59) return 0;
    offset_minutes = zone_hour * 60 + zone_minute;
    if (text[position - 6] == '-') offset_minutes = -offset_minutes;
  }
  if (position != length) return 0;
  int leap = (year % 4 == 0 && (year % 100 != 0 || year % 400 == 0));
  int month_days = month == 2 ? (leap ? 29 : 28) : ((month == 4 || month == 6 || month == 9 || month == 11) ? 30 : 31);
  if (day > month_days) return 0;
  int64_t seconds = osiris_host_days_from_civil(year, (unsigned)month, (unsigned)day) * 86400 + hour * 3600 + minute * 60 + second - (int64_t)offset_minutes * 60;
  result->tv_sec = (time_t)seconds; result->tv_nsec = nanoseconds; return 1;
}

static int osiris_host_root_owner_value(OValue value, uid_t *result) {
  int64_t signed_value = 0;
  if (value.kind == O_INT) signed_value = value.as.i;
  else if (value.kind == O_UINT && value.as.u <= UINTMAX_MAX) {
    if (value.as.u > (uint64_t)UINT_MAX) return 0;
    *result = (uid_t)value.as.u; return 1;
  } else return 0;
  if (signed_value < -1 || (signed_value >= 0 && (uint64_t)signed_value > (uint64_t)UINT_MAX)) return 0;
  *result = signed_value < 0 ? (uid_t)-1 : (uid_t)signed_value; return 1;
}

static OValue osiris_host_root_chown(OValue root_value, OValue path_value, OValue uid_value, OValue gid_value, int link) {
  uid_t uid = (uid_t)-1, gid = (uid_t)-1;
  if (!osiris_host_root_name(root_value, path_value, link ? "root_lchown" : "root_chown", NULL, NULL) || !osiris_host_root_owner_value(uid_value, &uid) || !osiris_host_root_owner_value(gid_value, &gid)) return ofail("root_chown expects bounded uid and gid integers");
  if (link) {
    int parent_fd = -1; char *leaf = NULL;
    if (!osiris_host_root_parent_fd(root_value, path_value, "root_lchown", &parent_fd, &leaf)) return onull();
    int result = fchownat(parent_fd, leaf, uid, gid, AT_SYMLINK_NOFOLLOW);
    int saved = errno; close(parent_fd); free(leaf); errno = saved;
    if (result != 0) return osiris_host_errno("root_lchown");
    return obool(1);
  }
  OHostResource *root = osiris_host_resource_get(root_value, OSIRIS_HOST_ROOT, "root_chown");
  if (!root) return onull();
  const char *path = NULL; size_t length = 0;
  if (!osiris_host_string(path_value, &path, &length)) return ofail("root_chown expects a path");
  int fd = -1;
  if (!osiris_host_root_open_followed(root->fd, path, O_RDONLY | O_NONBLOCK, 0, "root_chown", &fd)) return onull();
  int result = fchown(fd, uid, gid);
  int saved = errno; close(fd); errno = saved;
  if (result != 0) return osiris_host_errno("root_chown");
  return obool(1);
}

static OValue osiris_host_root_chtimes(OValue root_value, OValue path_value, OValue access_value, OValue modification_value) {
  OHostResource *root = NULL; const char *path = NULL;
  if (!osiris_host_root_name(root_value, path_value, "root_chtimes", &root, &path)) return onull();
  struct timespec times[2];
  if (!osiris_host_parse_timestamp(access_value, &times[0]) || !osiris_host_parse_timestamp(modification_value, &times[1])) return ofail("root_chtimes expects RFC3339 timestamps");
  int fd = -1;
  if (!osiris_host_root_open_followed(root->fd, path, O_RDONLY | O_NONBLOCK, 0, "root_chtimes", &fd)) return onull();
  int result = futimens(fd, times);
  int saved = errno; close(fd); errno = saved;
  if (result != 0) return osiris_host_errno("root_chtimes");
  return obool(1);
}

static OValue osiris_host_root_fs(OValue value) {
  OHostResource *root = osiris_host_resource_get(value, OSIRIS_HOST_ROOT, "root_fs");
  if (!root) return onull();
  return osiris_host_resource_value(root);
}

static OValue osiris_host_root_close(OValue value) {
  OHostResource *root = osiris_host_resource_get(value, OSIRIS_HOST_ROOT, "root_close");
  if (!root) return onull();
  osiris_host_resource_close(root); return obool(1);
}

static OValue osiris_host_resource_deadline(OValue value, OValue timeout) {
  OHostResource *resource = (OHostResource *)((value.kind == O_RESOURCE) ? value.as.resource : NULL);
  if (!resource || resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) return ofail("host resource is closed");
  int64_t timeout_ms = timeout.kind == O_NULL ? -1 : (timeout.kind == O_INT ? timeout.as.i : (timeout.kind == O_UINT && timeout.as.u <= INT64_MAX ? (int64_t)timeout.as.u : -2));
  if (timeout_ms < -1) return ofail("deadline expects a non-negative integer or null");
  resource->deadline_ms = timeout_ms < 0 ? -1 : osiris_host_now_ms() + timeout_ms; return onull();
}

static OValue osiris_host_close_any(OValue value) {
  if (value.kind != O_RESOURCE || !value.as.resource) return ofail("close expects an owned host resource");
  uint64_t magic = *(const uint64_t *)value.as.resource;
#if defined(OSIRIS_NATIVE_NETWORK_COMPONENT)
  if (magic == OSIRIS_NET_MAGIC) return onet_close(value);
#endif
  OHostResource *resource = (OHostResource *)value.as.resource;
  if (resource->magic != OSIRIS_HOST_RESOURCE_MAGIC || resource->closed) return ofail("resource is closed or invalid");
  osiris_host_resource_close(resource); return obool(1);
}

#else
#if !defined(OSIRIS_WINDOWS_COMPONENT)
static OValue osiris_host_resource_unavailable(void) { return ofail("native host resources are unavailable on this platform"); }
#define OSIRIS_HOST_UNAVAILABLE(name) static OValue name(OValue first, ...) { (void)first; return osiris_host_resource_unavailable(); }
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_spawn)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_find)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_wait)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_state)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_kill)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_release)
OSIRIS_HOST_UNAVAILABLE(osiris_host_process_stream)
OSIRIS_HOST_UNAVAILABLE(osiris_host_pipe)
OSIRIS_HOST_UNAVAILABLE(osiris_host_standard_stream)
OSIRIS_HOST_UNAVAILABLE(osiris_host_fd)
OSIRIS_HOST_UNAVAILABLE(osiris_host_name)
OSIRIS_HOST_UNAVAILABLE(osiris_host_new_file)
OSIRIS_HOST_UNAVAILABLE(osiris_host_signal_subscribe)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_open)
OSIRIS_HOST_UNAVAILABLE(osiris_host_stat)
OSIRIS_HOST_UNAVAILABLE(osiris_host_read_dir)
OSIRIS_HOST_UNAVAILABLE(osiris_host_mkdir)
OSIRIS_HOST_UNAVAILABLE(osiris_host_mkdir_all)
OSIRIS_HOST_UNAVAILABLE(osiris_host_remove)
OSIRIS_HOST_UNAVAILABLE(osiris_host_remove_all)
OSIRIS_HOST_UNAVAILABLE(osiris_host_truncate)
OSIRIS_HOST_UNAVAILABLE(osiris_host_rename)
OSIRIS_HOST_UNAVAILABLE(osiris_host_chmod)
OSIRIS_HOST_UNAVAILABLE(osiris_host_readlink)
OSIRIS_HOST_UNAVAILABLE(osiris_host_link)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_write_string)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_open)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_create)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_open_file)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_read_file)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_write_file)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_stat)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_chown)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_chtimes)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_fs)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_readlink)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_link)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_mkdir)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_mkdir_all)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_remove)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_remove_all)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_rename)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_chmod)
OSIRIS_HOST_UNAVAILABLE(osiris_host_root_close)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_close)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_io)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_seek)
OSIRIS_HOST_UNAVAILABLE(osiris_host_file_sync)
OSIRIS_HOST_UNAVAILABLE(osiris_host_resource_deadline)
#undef OSIRIS_HOST_UNAVAILABLE
#endif
#endif
`

// nativeHostResourceRuntimeSymbols is the exact function-level integration
// contract. Dispatchers should preserve these argument orders and return
// structured OValue errors without invoking another backend.
var nativeHostResourceRuntimeSymbols = map[string]string{
	"process_spawn":      "osiris_host_process_spawn(command, arguments, options_or_null)",
	"start_process":      "osiris_host_process_spawn(command, arguments, options_or_null)",
	"find_process":       "osiris_host_process_find(pid)",
	"process_wait":       "osiris_host_process_wait(process, timeout_or_null)",
	"process_state":      "osiris_host_process_state(process)",
	"process_kill":       "osiris_host_process_kill(process, signal_or_null)",
	"process_signal":     "osiris_host_process_kill(process, signal)",
	"process_release":    "osiris_host_process_release(process)",
	"process_stdin":      "osiris_host_process_stream(process, 0)",
	"process_stdout":     "osiris_host_process_stream(process, 1)",
	"process_stderr":     "osiris_host_process_stream(process, 2)",
	"pipe":               "osiris_host_pipe()",
	"stdin":              "osiris_host_standard_stream(0)",
	"stdout":             "osiris_host_standard_stream(1)",
	"stderr":             "osiris_host_standard_stream(2)",
	"fd":                 "osiris_host_fd(file)",
	"name":               "osiris_host_name(file)",
	"new_file":           "osiris_host_new_file(fd, name)",
	"with_handle":        "osiris_host_with_handle(file, callback)",
	"syscall_conn":       "osiris_host_syscall_conn(file)",
	"signal_subscribe":   "osiris_host_signal_subscribe(signal)",
	"open":               "osiris_host_file_open(path, O_RDONLY, 0)",
	"create":             "osiris_host_file_open(path, O_RDWR|O_CREAT|O_TRUNC, mode)",
	"open_file":          "osiris_host_file_open(path, flags, mode)",
	"stat":               "osiris_host_stat(path, 1)",
	"lstat":              "osiris_host_stat(path, 0)",
	"read_dir":           "osiris_host_read_dir(path)",
	"mkdir":              "osiris_host_mkdir(path, mode, false)",
	"mkdir_all":          "osiris_host_mkdir(path, mode, true)",
	"remove":             "osiris_host_remove(path, false)",
	"remove_all":         "osiris_host_remove(path, true)",
	"truncate":           "osiris_host_truncate(path, size)",
	"rename":             "osiris_host_rename(old_path, new_path)",
	"chmod":              "osiris_host_chmod(path, mode)",
	"readlink":           "osiris_host_readlink(path)",
	"link":               "osiris_host_link(old_path, new_path, false)",
	"symlink":            "osiris_host_link(old_path, new_path, true)",
	"write_string":       "osiris_host_file_write_string(file, string, timeout_or_null)",
	"open_root":          "osiris_host_root_open(path)",
	"root_create":        "osiris_host_root_create(root, path)",
	"root_open":          "osiris_host_root_open_file(root, path, O_RDONLY, 0)",
	"root_open_file":     "osiris_host_root_open_file(root, path, flags, mode)",
	"root_read_file":     "osiris_host_root_read_file(root, path)",
	"root_write_file":    "osiris_host_root_write_file(root, path, data, mode)",
	"root_stat":          "osiris_host_root_stat(root, path, 1)",
	"root_lstat":         "osiris_host_root_stat(root, path, 0)",
	"root_chown":         "osiris_host_root_chown(root, path, uid, gid, 0)",
	"root_lchown":        "osiris_host_root_chown(root, path, uid, gid, 1)",
	"root_chtimes":       "osiris_host_root_chtimes(root, path, access_time, modification_time)",
	"root_fs":            "osiris_host_root_fs(root)",
	"root_readlink":      "osiris_host_root_readlink(root, path)",
	"root_link":          "osiris_host_root_link(root, old_path, new_path, false)",
	"root_symlink":       "osiris_host_root_link(root, old_path, new_path, true)",
	"root_mkdir":         "osiris_host_root_mkdir(root, path, mode)",
	"root_mkdir_all":     "osiris_host_root_mkdir_all(root, path, mode)",
	"root_remove":        "osiris_host_root_remove(root, path)",
	"root_remove_all":    "osiris_host_root_remove_all(root, path)",
	"root_rename":        "osiris_host_root_rename(root, old_path, new_path)",
	"root_chmod":         "osiris_host_root_chmod(root, path, mode)",
	"root_close":         "osiris_host_root_close(root)",
	"close":              "osiris_host_file_close(file)",
	"read":               "osiris_host_file_io(file, size, null, timeout_or_null, 0, 0)",
	"write":              "osiris_host_file_io(file, bytes, null, timeout_or_null, 1, 0)",
	"read_at":            "osiris_host_file_io(file, size, offset, timeout_or_null, 0, 1)",
	"write_at":           "osiris_host_file_io(file, bytes, offset, timeout_or_null, 1, 1)",
	"seek_file":          "osiris_host_file_seek(file, offset, whence)",
	"sync":               "osiris_host_file_sync(file)",
	"set_deadline":       "osiris_host_resource_deadline(resource, timeout_or_null)",
	"set_read_deadline":  "osiris_host_resource_deadline(resource, timeout_or_null)",
	"set_write_deadline": "osiris_host_resource_deadline(resource, timeout_or_null)",
}

func nativeHostResourceIntegrationSummary() string {
	return strings.Join([]string{
		"append nativeHostResourceRuntimeComponent after cRuntimePrelude() and before generated declarations",
		"add nativeHostResourceMembers to nativeStaticModuleMember",
		"call nativeHostResourceCapability from native module capability validation",
		"emit calls using nativeHostResourceRuntimeSymbols without shell or backend fallback",
	}, "\n")
}
