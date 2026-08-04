package osiris

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeTLSHTTPRuntimeContract(t *testing.T) {
	wantMembers := []string{"tls_dial", "tls_dial_config", "tls_listen", "http_get", "http_request", "http_request_stream", "http_serve"}
	for _, member := range wantMembers {
		if _, ok := nativeTLSHTTPRuntimeSymbols[member]; !ok {
			t.Fatalf("native TLS/HTTP member %q is absent", member)
		}
	}
	for _, marker := range []string{
		"OirisNativeTLSProvider",
		"OirisNativeHTTPTransportProvider",
		"osiris_native_tls_dial",
		"osiris_native_tls_listen",
		"osiris_native_http_get",
		"osiris_native_http_request",
		"osiris_native_http_request_stream_value",
		"osiris_native_http_body_read",
		"osiris_native_http_serve",
		"OSIRIS_TLS_HTTP_MAX_BODY",
		"OSIRIS_TLS_HTTP_MAX_HEADERS",
		"osiris_tls_http_cancelled",
		"osiris_http_map_constructor",
		"Transfer-Encoding",
		"Content-Length",
		"OSIRIS_TLS_CAP_DIAL",
		"OSIRIS_HTTP_CAP_CLIENT",
	} {
		if !strings.Contains(cRuntimeTLSHTTPComponent, marker) {
			t.Fatalf("native TLS/HTTP component is missing %q", marker)
		}
	}
}

func TestNativeTLSHTTPRuntimeCompilesAsIsolatedCComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the isolated provider contract uses generated POSIX-compatible C headers")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not installed")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "tls_http.c")
	source := nativeTLSHTTPSyntaxPrelude + "\n" + cRuntimeTLSHTTPComponent + "\nint main(void) { return 0; }\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", "-fsyntax-only", sourcePath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated native TLS/HTTP component does not compile: %v\n%s", err, output)
	}
}

func TestNativeTLSHTTPRuntimeExecutesHTTPProviderContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the focused provider fixture uses the portable C ABI only on POSIX test hosts")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not installed")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "tls_http_provider.c")
	executablePath := filepath.Join(dir, "tls_http_provider")
	source := nativeTLSHTTPSyntaxPrelude + "\n" + cRuntimeTLSHTTPComponent + `
typedef struct { const char *input; size_t input_length; size_t input_offset; char output[4096]; size_t output_length; int closed; } FixtureStream;
static int fixture_cancel(void *userdata) { (void)userdata; return 0; }
static int64_t fixture_read(void *state, unsigned char *buffer, size_t capacity, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) {
  (void)timeout; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (cancel && cancel(userdata)) return -1; if (stream->input_offset == stream->input_length) return 0; size_t count = stream->input_length - stream->input_offset; if (count > capacity) count = capacity; memcpy(buffer, stream->input + stream->input_offset, count); stream->input_offset += count; return (int64_t)count;
}
static int64_t fixture_write(void *state, const unsigned char *buffer, size_t length, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) {
  (void)timeout; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (cancel && cancel(userdata)) return -1; if (length > sizeof(stream->output) - stream->output_length) return -1; memcpy(stream->output + stream->output_length, buffer, length); stream->output_length += length; return (int64_t)length;
}
static int fixture_close(void *state, char *error, size_t error_capacity) { (void)error; (void)error_capacity; ((FixtureStream *)state)->closed = 1; return 0; }
static void fixture_destroy(void *state) { free(state); }
static const char fixture_response[] = "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nX-Test: yes\r\n\r\nhello";
static int fixture_dial(void *userdata, const char *address, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) {
  (void)userdata; (void)timeout; (void)error; (void)error_capacity; if (strcmp(address, "example.test:80") != 0) return -1; FixtureStream *state = (FixtureStream *)calloc(1, sizeof(*state)); if (!state) return -1; state->input = fixture_response; state->input_length = sizeof(fixture_response) - 1; stream->state = state; stream->read = fixture_read; stream->write = fixture_write; stream->close = fixture_close; stream->destroy = fixture_destroy; return 0;
}
static OValue fixture_map(void *userdata, const OirisHTTPMapEntry *entries, size_t count) {
  (void)userdata; OValue *values = (OValue *)calloc(count * 2, sizeof(OValue)); if (!values) return ofail("fixture map allocation failed"); for (size_t i = 0; i < count; i++) { values[i * 2] = ostring(entries[i].key); values[i * 2 + 1] = entries[i].value; } return olist(values, count * 2);
}
static OirisNativeHTTPTransportProvider fixture_provider = {1, OSIRIS_HTTP_CAP_CLIENT, NULL, fixture_dial, NULL, NULL};
int main(void) {
  if (!osiris_native_tls_http_install(NULL, &fixture_provider, fixture_map, NULL, NULL, NULL, NULL)) return 10;
  OValue response = osiris_native_http_get(ostring("http://example.test/"), oint(1000));
  if (response.kind != O_LIST || response.as.list.len != 8) { fprintf(stderr, "kind=%d len=%zu error=%s\\n", response.kind, response.kind == O_LIST ? response.as.list.len : 0u, oerrormessage ? oerrormessage : ""); return 11; }
  return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", sourcePath, "-o", executablePath)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("provider fixture does not compile: %v\n%s", err, output)
	}
	if output, err := exec.Command(executablePath).CombinedOutput(); err != nil {
		t.Fatalf("provider-backed HTTP fixture failed: %v\n%s", err, output)
	}
}

func TestNativeTLSHTTPRuntimeFollowsBoundedRedirects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the focused provider fixture uses the portable C ABI only on POSIX test hosts")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not installed")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "tls_http_redirect.c")
	executablePath := filepath.Join(dir, "tls_http_redirect")
	source := nativeTLSHTTPSyntaxPrelude + "\n" + cRuntimeTLSHTTPComponent + `
typedef struct { const char *input; size_t input_length; size_t input_offset; char output[4096]; size_t output_length; int closed; } FixtureStream;
static int fixture_mode;
static int fixture_calls;
static int fixture_closed;
static int fixture_post_seen;
static int fixture_get_seen;
static const char fixture_redirect[] = "HTTP/1.1 302 Found\r\nLocation: /final\r\nContent-Length: 0\r\n\r\n";
static const char fixture_final[] = "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok";
static int64_t fixture_read(void *state, unsigned char *buffer, size_t capacity, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) {
  (void)timeout; (void)cancel; (void)userdata; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (stream->input_offset == stream->input_length) return 0; size_t count = stream->input_length - stream->input_offset; if (count > capacity) count = capacity; memcpy(buffer, stream->input + stream->input_offset, count); stream->input_offset += count; return (int64_t)count;
}
static int64_t fixture_write(void *state, const unsigned char *buffer, size_t length, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) {
  (void)timeout; (void)cancel; (void)userdata; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (length > sizeof(stream->output) - stream->output_length - 1) return -1; memcpy(stream->output + stream->output_length, buffer, length); stream->output_length += length; stream->output[stream->output_length] = 0; return (int64_t)length;
}
static int fixture_close(void *state, char *error, size_t error_capacity) { (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; stream->closed = 1; if (strstr(stream->output, "POST /start")) fixture_post_seen = 1; if (strstr(stream->output, "GET /final")) fixture_get_seen = 1; fixture_closed++; return 0; }
static void fixture_destroy(void *state) { free(state); }
static int fixture_dial(void *userdata, const char *address, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) {
  (void)userdata; (void)timeout; (void)error; (void)error_capacity; if (strcmp(address, "example.test:80") != 0) return -1; FixtureStream *state = (FixtureStream *)calloc(1, sizeof(*state)); if (!state) return -1; fixture_calls++; if (fixture_mode == 1 || fixture_calls == 1) { state->input = fixture_redirect; state->input_length = sizeof(fixture_redirect) - 1; } else { state->input = fixture_final; state->input_length = sizeof(fixture_final) - 1; } stream->state = state; stream->read = fixture_read; stream->write = fixture_write; stream->close = fixture_close; stream->destroy = fixture_destroy; return 0;
}
static OValue fixture_map(void *userdata, const OirisHTTPMapEntry *entries, size_t count) { (void)userdata; OValue *values = (OValue *)calloc(count * 2, sizeof(OValue)); if (!values) return ofail("fixture map allocation failed"); for (size_t i = 0; i < count; i++) { values[i * 2] = ostring(entries[i].key); values[i * 2 + 1] = entries[i].value; } return olist(values, count * 2); }
static OirisNativeHTTPTransportProvider fixture_provider = {1, OSIRIS_HTTP_CAP_CLIENT, NULL, fixture_dial, NULL, NULL};
int main(void) {
  if (!osiris_native_tls_http_install(NULL, &fixture_provider, fixture_map, NULL, NULL, NULL, NULL)) return 10;
  OValue response = osiris_native_http_request(ostring("POST"), ostring("http://example.test/start"), onull(), ostring("payload"), oint(1000));
  if (response.kind != O_LIST || response.as.list.len != 8 || fixture_calls != 2 || fixture_closed != 2 || !fixture_post_seen || !fixture_get_seen) { fprintf(stderr, "redirect first kind=%d len=%zu calls=%d closed=%d post=%d get=%d err=%s\\n", response.kind, response.kind == O_LIST ? response.as.list.len : 0u, fixture_calls, fixture_closed, fixture_post_seen, fixture_get_seen, oerrormessage ? oerrormessage : ""); return 11; }
  if (response.as.list.items[7].kind != O_STRING || strcmp(response.as.list.items[7].as.s.data, "http://example.test/final") != 0) return 12;
  fixture_mode = 1; fixture_calls = 0; fixture_closed = 0; ohaderror = 0;
  response = osiris_native_http_get(ostring("http://example.test/loop"), oint(1000));
  if (response.kind != O_NULL || fixture_calls != 11 || fixture_closed != 11 || !ohaderror || !oerrormessage || strstr(oerrormessage, "redirect limit") == NULL) return 14;
  return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", sourcePath, "-o", executablePath).CombinedOutput(); err != nil {
		t.Fatalf("redirect fixture does not compile: %v\n%s", err, output)
	}
	if output, err := exec.Command(executablePath).CombinedOutput(); err != nil {
		t.Fatalf("redirect fixture failed: %v\n%s", err, output)
	}
}

func TestNativeTLSHTTPRuntimeExecutesGeneratedClosureHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the focused provider fixture uses the portable C ABI only on POSIX test hosts")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not installed")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "tls_http_closure.c")
	executablePath := filepath.Join(dir, "tls_http_closure")
	source := nativeTLSHTTPSyntaxPrelude + "\n" + cRuntimeTLSHTTPComponent + `
typedef struct { const char *input; size_t input_length; size_t input_offset; char output[4096]; size_t output_length; int closed; } FixtureStream;
static int fixture_served;
static int fixture_done;
static int fixture_handler_calls;
static int fixture_handler_fail;
static int fixture_closed;
static const char fixture_request[] = "GET /hello HTTP/1.1\r\nHost: example.test\r\n\r\n";
static int fixture_cancel(void *userdata) { (void)userdata; return fixture_done; }
static int64_t fixture_read(void *state, unsigned char *buffer, size_t capacity, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) { (void)timeout; (void)cancel; (void)userdata; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (stream->input_offset == stream->input_length) return 0; size_t count = stream->input_length - stream->input_offset; if (count > capacity) count = capacity; memcpy(buffer, stream->input + stream->input_offset, count); stream->input_offset += count; return (int64_t)count; }
static int64_t fixture_write(void *state, const unsigned char *buffer, size_t length, int64_t timeout, OtlsCancel cancel, void *userdata, char *error, size_t error_capacity) { (void)timeout; (void)cancel; (void)userdata; (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; if (length > sizeof(stream->output) - stream->output_length - 1) return -1; memcpy(stream->output + stream->output_length, buffer, length); stream->output_length += length; stream->output[stream->output_length] = 0; return (int64_t)length; }
static int fixture_close(void *state, char *error, size_t error_capacity) { (void)error; (void)error_capacity; FixtureStream *stream = (FixtureStream *)state; stream->closed = 1; fixture_closed++; return 0; }
static void fixture_destroy(void *state) { free(state); }
static int fixture_listen(void *userdata, const char *address, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) { (void)userdata; (void)address; (void)error; (void)error_capacity; FixtureStream *state = (FixtureStream *)calloc(1, sizeof(*state)); if (!state) return -1; stream->state = state; stream->close = fixture_close; stream->destroy = fixture_destroy; return 0; }
static int fixture_accept(void *userdata, OirisTLSHTTPStream *listener, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) { (void)userdata; (void)listener; (void)timeout; (void)error; (void)error_capacity; FixtureStream *state = (FixtureStream *)calloc(1, sizeof(*state)); if (!state) return -1; state->input = fixture_request; state->input_length = sizeof(fixture_request) - 1; fixture_served = 1; stream->state = state; stream->read = fixture_read; stream->write = fixture_write; stream->close = fixture_close; stream->destroy = fixture_destroy; return 0; }
static OValue fixture_map(void *userdata, const OirisHTTPMapEntry *entries, size_t count) { (void)userdata; OValue *values = (OValue *)calloc(count * 2, sizeof(OValue)); if (!values) return ofail("fixture map allocation failed"); for (size_t i = 0; i < count; i++) { values[i * 2] = ostring(entries[i].key); values[i * 2 + 1] = entries[i].value; } return olist(values, count * 2); }
static int fixture_lookup(void *userdata, OValue map, const char *key, OValue *output) { (void)userdata; if (map.kind != O_LIST || (map.as.list.len % 2) != 0) return 0; for (size_t i = 0; i < map.as.list.len; i += 2) if (map.as.list.items[i].kind == O_STRING && strcmp(map.as.list.items[i].as.s.data, key) == 0) { *output = map.as.list.items[i + 1]; return 1; } return 0; }
static OValue fixture_handler(const OValue *args, size_t argc, const OValue *captures, size_t capture_count) { (void)captures; (void)capture_count; if (argc != 1 || args[0].kind != O_LIST) return ofail("handler request was not a map"); fixture_handler_calls++; fixture_done = 1; if (fixture_handler_fail) return ofail("generated handler failed"); OValue values[6] = {ostring("status"), oint(201), ostring("body"), ostring("closure-body"), ostring("headers"), olist(NULL, 0)}; return olist(values, 6); }
static OirisNativeHTTPTransportProvider fixture_provider = {1, OSIRIS_HTTP_CAP_SERVER, NULL, NULL, fixture_listen, fixture_accept};
int main(void) {
  if (!osiris_native_tls_http_install(NULL, &fixture_provider, fixture_map, NULL, fixture_lookup, NULL, NULL)) return 10;
  osiris_native_tls_http_set_cancel(fixture_cancel, NULL);
  OirisTLSHTTPStream listener_stream = {0}; if (fixture_listen(NULL, "example.test:80", &listener_stream, NULL, 0) != 0) return 11;
  OirisTLSHTTPResource *listener_resource = osiris_tls_http_new(OSIRIS_TLS_HTTP_RESOURCE_LISTENER, listener_stream, "fixture_listener"); if (!listener_resource) return 12;
  OValue closure = {0}; closure.kind = O_CLOSURE; closure.as.closure.call = fixture_handler;
  int result = osiris_native_http_serve(osiris_tls_http_resource_value(listener_resource), closure, onull());
  OValue listener_close = osiris_tls_http_close(osiris_tls_http_resource_value(listener_resource));
  if (result != -1 || listener_close.kind == O_NULL || fixture_handler_calls != 1 || fixture_closed < 2) { fprintf(stderr, "closure result=%d closekind=%d calls=%d closed=%d err=%s\\n", result, listener_close.kind, fixture_handler_calls, fixture_closed, oerrormessage ? oerrormessage : ""); return 13; }
  if (!ohaderror || !oerrormessage || strstr(oerrormessage, "cancelled") == NULL) return 14;
  fixture_done = 0; fixture_handler_fail = 1; fixture_served = 0; ohaderror = 0;
  listener_stream = (OirisTLSHTTPStream){0}; if (fixture_listen(NULL, "example.test:80", &listener_stream, NULL, 0) != 0) return 15;
  listener_resource = osiris_tls_http_new(OSIRIS_TLS_HTTP_RESOURCE_LISTENER, listener_stream, "fixture_listener"); if (!listener_resource) return 16;
  result = osiris_native_http_serve(osiris_tls_http_resource_value(listener_resource), closure, onull());
  listener_close = osiris_tls_http_close(osiris_tls_http_resource_value(listener_resource));
  if (result != -1 || listener_close.kind == O_NULL || !ohaderror || !oerrormessage || strstr(oerrormessage, "generated handler failed") == NULL) return 17;
  return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(cc, "-std=c11", "-Werror", "-Wno-switch", sourcePath, "-o", executablePath).CombinedOutput(); err != nil {
		t.Fatalf("closure fixture does not compile: %v\n%s", err, output)
	}
	if output, err := exec.Command(executablePath).CombinedOutput(); err != nil {
		t.Fatalf("closure fixture failed: %v\n%s", err, output)
	}
}

// Keep this syntax test independent of unrelated changes in the large shared
// native prelude. The production component is compiled against that prelude at
// integration time; these are the exact value/helper fields it consumes.
const nativeTLSHTTPSyntaxPrelude = `
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
typedef enum { O_NULL, O_BOOL, O_UINT, O_INT, O_STRING, O_LIST, O_CLOSURE, O_ERROR, O_RESOURCE } OKind;
typedef struct OValue OValue;
typedef OValue (*OCall)(const OValue *args, size_t argc, const OValue *captures, size_t capture_count);
typedef struct { const char *data; size_t len; } OString;
typedef struct { OValue *items; size_t len; } OList;
typedef struct { OString kind; OString message; OString operation; OString resource; int retryable; } OError;
typedef struct { OCall call; OValue *captures; size_t capture_count; } OClosure;
struct OValue { OKind kind; uint16_t fbits; union { int b; uint64_t u; int64_t i; OString s; OList list; OError error; OClosure closure; void *resource; } as; };
static OValue olasterror;
static int ohaderror;
static const char *oerrormessage;
static int otrack(void *value) { return value != NULL; }
static OValue onull(void) { OValue value = {0}; value.kind = O_NULL; return value; }
static OValue obool(int value) { OValue result = {0}; result.kind = O_BOOL; result.as.b = value != 0; return result; }
static OValue ouintbits(uint64_t value, uint16_t bits) { OValue result = {0}; result.kind = O_UINT; result.fbits = bits; result.as.u = value; return result; }
static OValue oint(int64_t value) { OValue result = {0}; result.kind = O_INT; result.as.i = value; return result; }
static OValue ostring(const char *value) { OValue result = {0}; result.kind = O_STRING; result.as.s.data = value; result.as.s.len = strlen(value); return result; }
static OValue ostringview(const char *value, size_t length) { OValue result = {0}; result.kind = O_STRING; result.as.s.data = value; result.as.s.len = length; return result; }
static OValue olist(const OValue *items, size_t length) { OValue result = {0}; result.kind = O_LIST; result.as.list.items = (OValue *)items; result.as.list.len = length; return result; }
static OValue oerrorvalue(const char *kind, const char *message) { OValue result = {0}; result.kind = O_ERROR; result.as.error.kind = (OString){kind, strlen(kind)}; result.as.error.message = (OString){message, strlen(message)}; return result; }
static OValue ofail(const char *message) { return oerrorvalue("native", message); }
static OValue ocall(OValue function, const OValue *args, size_t argc) { if (function.kind != O_CLOSURE || !function.as.closure.call) return ofail("value is not callable"); return function.as.closure.call(args, argc, function.as.closure.captures, function.as.closure.capture_count); }
`
