package osiris

// cRuntimeTLSHTTPComponent is the native-C TLS/HTTP host-resource ABI.
//
// The generated C value ABI must not silently reuse O_FFI for network resources. This
// component consequently exposes a small, explicit provider boundary. A
// platform provider (OpenSSL, Secure Transport, Schannel, or another audited
// implementation) supplies verified TLS and socket operations; the component
// owns the resulting resource, applies the language limits, performs HTTP/1.1
// framing and parsing, and calls an installed map/handler bridge for language
// values. It is executable when those providers are installed; absent
// providers produce structured capability errors rather than fallback calls.
const cRuntimeTLSHTTPComponent = `
#include <ctype.h>
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define OSIRIS_TLS_HTTP_MAGIC UINT64_C(0x4f5352544c534854)
#define OSIRIS_TLS_HTTP_MAX_BODY (8u * 1024u * 1024u)
#define OSIRIS_TLS_HTTP_MAX_HEADERS 256u
#define OSIRIS_TLS_HTTP_MAX_LINE 8192u
#define OSIRIS_TLS_HTTP_MAX_URL 16384u
#define OSIRIS_TLS_HTTP_MAX_REQUEST (OSIRIS_TLS_HTTP_MAX_BODY + (256u * 1024u))
#define OSIRIS_TLS_HTTP_MAX_REDIRECTS 10u

enum {
  OSIRIS_TLS_CAP_DIAL = UINT64_C(1) << 0,
  OSIRIS_TLS_CAP_LISTEN = UINT64_C(1) << 1,
  OSIRIS_HTTP_CAP_CLIENT = UINT64_C(1) << 2,
  OSIRIS_HTTP_CAP_STREAM = UINT64_C(1) << 3,
  OSIRIS_HTTP_CAP_SERVER = UINT64_C(1) << 4
};

typedef int (*OtlsCancel)(void *userdata);
typedef struct {
  void *state;
  int64_t (*read)(void *state, unsigned char *buffer, size_t capacity,
                  int64_t timeout_ms, OtlsCancel cancelled, void *userdata,
                  char *error, size_t error_capacity);
  int64_t (*write)(void *state, const unsigned char *buffer, size_t length,
                   int64_t timeout_ms, OtlsCancel cancelled, void *userdata,
                   char *error, size_t error_capacity);
  int (*close)(void *state, char *error, size_t error_capacity);
  void (*destroy)(void *state);
} OirisTLSHTTPStream;

typedef struct {
  uint32_t abi_version;
  uint64_t capabilities;
  void *userdata;
  int (*dial)(void *userdata, const char *address, const char *server_name,
              const char *roots_pem, int64_t timeout_ms,
              OirisTLSHTTPStream *stream, char *error, size_t error_capacity);
  int (*listen)(void *userdata, const char *address, const char *certificate,
                const char *key, OirisTLSHTTPStream *listener,
                char *error, size_t error_capacity);
  int (*accept)(void *userdata, OirisTLSHTTPStream *listener,
                int64_t timeout_ms, OirisTLSHTTPStream *stream,
                char *error, size_t error_capacity);
} OirisNativeTLSProvider;

typedef struct {
  uint32_t abi_version;
  uint64_t capabilities;
  void *userdata;
  int (*dial)(void *userdata, const char *address, int64_t timeout_ms,
              OirisTLSHTTPStream *stream, char *error, size_t error_capacity);
  int (*listen)(void *userdata, const char *address,
                OirisTLSHTTPStream *listener, char *error, size_t error_capacity);
  int (*accept)(void *userdata, OirisTLSHTTPStream *listener,
                int64_t timeout_ms, OirisTLSHTTPStream *stream,
                char *error, size_t error_capacity);
} OirisNativeHTTPTransportProvider;

typedef struct {
  const char *key;
  OValue value;
} OirisHTTPMapEntry;

typedef OValue (*OirisHTTPMapConstructor)(void *userdata,
                                           const OirisHTTPMapEntry *entries,
                                           size_t count);
typedef int (*OirisHTTPMapReader)(void *userdata, OValue value,
                                   int (*entry)(void *entry_userdata,
                                                const char *key, size_t key_len,
                                                OValue value),
                                   void *entry_userdata);
typedef int (*OirisHTTPHandler)(void *userdata, OValue request,
                                OValue *response, char *error,
                                size_t error_capacity);
typedef int (*OirisHTTPMapLookup)(void *userdata, OValue map, const char *key,
                                  OValue *value);

typedef enum {
  OSIRIS_TLS_HTTP_RESOURCE_STREAM = 1,
  OSIRIS_TLS_HTTP_RESOURCE_LISTENER = 2,
  OSIRIS_TLS_HTTP_RESOURCE_BODY = 3
} OirisTLSHTTPResourceKind;

typedef struct {
  uint64_t magic;
  OirisTLSHTTPResourceKind kind;
	int closed;
	int secure_listener;
  OirisTLSHTTPStream stream;
  int chunked;
  int64_t remaining;
  int64_t chunk_remaining;
  int chunk_done;
  unsigned char buffer[4096];
  size_t buffered;
  size_t buffer_offset;
} OirisTLSHTTPResource;

typedef struct {
  char *name;
  char *value;
} OirisHTTPHeader;

typedef struct {
  int secure;
  char host[OSIRIS_TLS_HTTP_MAX_URL];
  char port[16];
  char target[OSIRIS_TLS_HTTP_MAX_URL];
  char authority[OSIRIS_TLS_HTTP_MAX_URL];
} OirisHTTPURL;

static const OirisNativeTLSProvider *osiris_tls_provider = NULL;
static const OirisNativeHTTPTransportProvider *osiris_http_transport_provider = NULL;
static OirisHTTPMapConstructor osiris_http_map_constructor = NULL;
static OirisHTTPMapReader osiris_http_map_reader = NULL;
static OirisHTTPMapLookup osiris_http_map_lookup = NULL;
static OirisHTTPHandler osiris_http_handler = NULL;
static void *osiris_tls_http_userdata = NULL;
static OtlsCancel osiris_tls_http_cancel = NULL;
static void *osiris_tls_http_cancel_userdata = NULL;
#ifdef OSIRIS_OPENSSL_PROVIDER
static const OirisNativeTLSProvider *osiris_native_openssl_provider(void);
#endif

static void osiris_tls_http_copy_error(char *destination, size_t capacity,
                                       const char *source) {
  if (!destination || capacity == 0) return;
  if (!source) source = "native TLS/HTTP provider failed";
  size_t length = strlen(source);
  if (length >= capacity) length = capacity - 1;
  memcpy(destination, source, length);
  destination[length] = 0;
}

static OValue osiris_tls_http_error(const char *operation, const char *resource,
                                    const char *detail) {
  OValue error = oerrorvalue("network", detail ? detail : operation);
  if (error.kind == O_ERROR) {
    error.as.error.operation = (OString){operation, strlen(operation)};
    error.as.error.resource = (OString){resource ? resource : "",
                                       resource ? strlen(resource) : 0};
    error.as.error.retryable = 1;
  }
  olasterror = error;
  ohaderror = 1;
  oerrormessage = detail ? detail : operation;
  return onull();
}

static int osiris_tls_http_cancelled(void) {
  return osiris_tls_http_cancel &&
         osiris_tls_http_cancel(osiris_tls_http_cancel_userdata);
}

static OValue osiris_tls_http_cancel_error(const char *operation) {
  return osiris_tls_http_error(operation, "", "operation cancelled");
}

static int osiris_tls_http_string(OValue value, const char **data,
                                  size_t *length, const char *operation) {
  if (value.kind != O_STRING || !value.as.s.data ||
      value.as.s.len > OSIRIS_TLS_HTTP_MAX_URL ||
      memchr(value.as.s.data, 0, value.as.s.len) != NULL) {
    osiris_tls_http_error(operation, "", "argument must be a bounded NUL-free string");
    return 0;
  }
  *data = value.as.s.data;
  *length = value.as.s.len;
  return 1;
}

static int osiris_tls_http_timeout(OValue value, int64_t *timeout,
                                   const char *operation) {
  if (value.kind == O_NULL) { *timeout = 0; return 1; }
  if (value.kind == O_UINT && value.as.u <= INT64_MAX) {
    *timeout = (int64_t)value.as.u;
    return 1;
  }
  if (value.kind == O_INT && value.as.i >= 0) {
    *timeout = value.as.i;
    return 1;
  }
  osiris_tls_http_error(operation, "", "timeout must be a non-negative integer or null");
  return 0;
}

static OirisTLSHTTPResource *osiris_tls_http_resource(OValue value,
                                                       OirisTLSHTTPResourceKind kind,
                                                       const char *operation) {
  if (value.kind != O_RESOURCE || !value.as.resource) {
    osiris_tls_http_error(operation, "", "expected an owned native TLS/HTTP resource");
    return NULL;
  }
  OirisTLSHTTPResource *resource = (OirisTLSHTTPResource *)value.as.resource;
	if (resource->magic != OSIRIS_TLS_HTTP_MAGIC || resource->closed ||
		resource->kind != kind || !resource->stream.state ||
		(kind != OSIRIS_TLS_HTTP_RESOURCE_LISTENER && !resource->stream.read)) {
    osiris_tls_http_error(operation, "", "TLS/HTTP resource is closed or has the wrong kind");
    return NULL;
  }
  return resource;
}

static OValue osiris_tls_http_resource_value(OirisTLSHTTPResource *resource) {
  return (OValue){O_RESOURCE, 0, {.resource = resource}};
}

static OValue osiris_tls_http_owned_string(const unsigned char *data, size_t length,
                                           const char *operation) {
  char *copy = (char *)malloc(length + 1);
  if (!copy) return osiris_tls_http_error(operation, "", "out of memory");
  if (length) memcpy(copy, data, length);
  copy[length] = 0;
  if (!otrack(copy)) { free(copy); return osiris_tls_http_error(operation, "", "out of memory"); }
  return ostringview(copy, length);
}

static OirisTLSHTTPResource *osiris_tls_http_new(OirisTLSHTTPResourceKind kind,
                                                  OirisTLSHTTPStream stream,
                                                  const char *operation) {
	if (!stream.state || !stream.close ||
		(kind != OSIRIS_TLS_HTTP_RESOURCE_LISTENER && (!stream.read || !stream.write))) {
    osiris_tls_http_error(operation, "", "provider returned an incomplete owned stream");
    if (stream.destroy && stream.state) stream.destroy(stream.state);
    return NULL;
  }
  OirisTLSHTTPResource *resource = (OirisTLSHTTPResource *)calloc(1, sizeof(*resource));
  if (!resource || !otrack(resource)) {
    free(resource);
    stream.close(stream.state, NULL, 0);
    if (stream.destroy) stream.destroy(stream.state);
    osiris_tls_http_error(operation, "", "out of memory");
    return NULL;
  }
  resource->magic = OSIRIS_TLS_HTTP_MAGIC;
  resource->kind = kind;
  resource->stream = stream;
  return resource;
}

static OValue osiris_tls_http_close(OValue value) {
  if (value.kind != O_RESOURCE || !value.as.resource) {
    return osiris_tls_http_error("close", "", "expected an owned native TLS/HTTP resource");
  }
  OirisTLSHTTPResource *resource = (OirisTLSHTTPResource *)value.as.resource;
  if (resource->magic != OSIRIS_TLS_HTTP_MAGIC || resource->closed) {
    return osiris_tls_http_error("close", "", "TLS/HTTP resource is already closed");
  }
  resource->closed = 1;
  char error[256] = {0};
  int result = resource->stream.close(resource->stream.state, error, sizeof(error));
  if (resource->stream.destroy) resource->stream.destroy(resource->stream.state);
  resource->stream.state = NULL;
  if (result != 0) return osiris_tls_http_error("close", "", error[0] ? error : "provider close failed");
  return obool(1);
}

static int osiris_tls_http_write_all(OirisTLSHTTPResource *resource,
                                     const unsigned char *data, size_t length,
                                     int64_t timeout, const char *operation) {
  size_t offset = 0;
  while (offset < length) {
    if (osiris_tls_http_cancelled()) { osiris_tls_http_cancel_error(operation); return 0; }
    char error[256] = {0};
    int64_t written = resource->stream.write(resource->stream.state, data + offset,
                                              length - offset, timeout,
                                              osiris_tls_http_cancel,
                                              osiris_tls_http_cancel_userdata,
                                              error, sizeof(error));
    if (written <= 0 || (uint64_t)written > length - offset) {
      osiris_tls_http_error(operation, "", error[0] ? error : "provider write failed");
      return 0;
    }
    offset += (size_t)written;
  }
  return 1;
}

static int64_t osiris_tls_http_read_raw(OirisTLSHTTPResource *resource,
                                        unsigned char *buffer, size_t capacity,
                                        int64_t timeout, const char *operation) {
  if (osiris_tls_http_cancelled()) { osiris_tls_http_cancel_error(operation); return -2; }
  char error[256] = {0};
  int64_t result = resource->stream.read(resource->stream.state, buffer, capacity,
                                          timeout, osiris_tls_http_cancel,
                                          osiris_tls_http_cancel_userdata,
                                          error, sizeof(error));
  if (result < 0) {
    osiris_tls_http_error(operation, "", error[0] ? error : "provider read failed");
    return -1;
  }
  if ((uint64_t)result > capacity) {
    osiris_tls_http_error(operation, "", "provider returned an oversized read");
    return -1;
  }
  return result;
}

static int osiris_tls_http_fill(OirisTLSHTTPResource *resource, int64_t timeout,
                                const char *operation) {
  if (resource->buffer_offset < resource->buffered) return 1;
  int64_t count = osiris_tls_http_read_raw(resource, resource->buffer,
                                           sizeof(resource->buffer), timeout, operation);
  if (count < 0) return 0;
  resource->buffered = (size_t)count;
  resource->buffer_offset = 0;
  return count != 0;
}

static int osiris_tls_http_get_byte(OirisTLSHTTPResource *resource, unsigned char *out,
                                    int64_t timeout, const char *operation) {
  if (!osiris_tls_http_fill(resource, timeout, operation)) return 0;
  *out = resource->buffer[resource->buffer_offset++];
  return 1;
}

static int osiris_tls_http_line(OirisTLSHTTPResource *resource, char *line,
                                size_t capacity, int64_t timeout,
                                const char *operation) {
  size_t length = 0;
  unsigned char byte = 0;
  while (length + 1 < capacity) {
    if (!osiris_tls_http_get_byte(resource, &byte, timeout, operation)) return 0;
    if (byte == '\n') {
      if (length && line[length - 1] == '\r') length--;
      line[length] = 0;
      return 1;
    }
    if (byte == 0 || iscntrl(byte) && byte != '\t' && byte != '\r') {
      osiris_tls_http_error(operation, "", "HTTP line contains an invalid control byte");
      return 0;
    }
    line[length++] = (char)byte;
  }
  osiris_tls_http_error(operation, "", "HTTP line exceeds the bounded header limit");
  return 0;
}

static int osiris_tls_http_header_name(const char *name, size_t length) {
  if (length == 0) return 0;
  for (size_t i = 0; i < length; i++) {
    unsigned char c = (unsigned char)name[i];
    if (!(isalnum(c) || c == '!' || c == '#' || c == '$' || c == '%' || c == '&' ||
          c == '\'' || c == '*' || c == '+' || c == '-' || c == '.' || c == '^' ||
			c == '_' || c == 96 || c == '|' || c == '~')) return 0;
  }
  return 1;
}

static int osiris_tls_http_header_value(const char *value, size_t length) {
  for (size_t i = 0; i < length; i++) {
    unsigned char c = (unsigned char)value[i];
    if (c == '\r' || c == '\n' || c == 0 || (c < 32 && c != '\t')) return 0;
  }
  return 1;
}

static int osiris_tls_http_case_equal(const char *left, const char *right) {
	while (*left && *right) {
		unsigned char a = (unsigned char)*left++;
		unsigned char b = (unsigned char)*right++;
		if (tolower(a) != tolower(b)) return 0;
	}
	return *left == 0 && *right == 0;
}

static int osiris_tls_http_case_equal_n(const char *left, size_t length, const char *right) {
  size_t right_length = strlen(right);
  if (length != right_length) return 0;
  for (size_t i = 0; i < length; i++) {
    if (tolower((unsigned char)left[i]) != tolower((unsigned char)right[i])) return 0;
  }
  return 1;
}

static void osiris_tls_http_free_headers(OirisHTTPHeader *headers, size_t count) {
  for (size_t i = 0; i < count; i++) { free(headers[i].name); free(headers[i].value); }
}

static int osiris_tls_http_add_header(OirisHTTPHeader *headers, size_t *count,
                                      const char *name, size_t name_length,
                                      const char *value, size_t value_length) {
  if (*count >= OSIRIS_TLS_HTTP_MAX_HEADERS || !osiris_tls_http_header_name(name, name_length) ||
      !osiris_tls_http_header_value(value, value_length)) return 0;
  char *name_copy = (char *)malloc(name_length + 1);
  char *value_copy = (char *)malloc(value_length + 1);
  if (!name_copy || !value_copy) { free(name_copy); free(value_copy); return 0; }
  memcpy(name_copy, name, name_length); name_copy[name_length] = 0;
  memcpy(value_copy, value, value_length); value_copy[value_length] = 0;
  headers[*count] = (OirisHTTPHeader){name_copy, value_copy};
  (*count)++;
  return 1;
}

typedef struct { OirisHTTPHeader *headers; size_t count; int failed; } OirisHTTPHeaderCollector;
static int osiris_tls_http_collect_header(void *userdata, const char *key, size_t key_len,
                                          OValue value) {
  OirisHTTPHeaderCollector *collector = (OirisHTTPHeaderCollector *)userdata;
  const char *data = NULL; size_t length = 0;
  if (!osiris_tls_http_string(value, &data, &length, "http_request")) { collector->failed = 1; return 0; }
  if (!osiris_tls_http_add_header(collector->headers, &collector->count, key, key_len, data, length)) {
    collector->failed = 1; return 0;
  }
  return 1;
}

static int osiris_tls_http_headers(OValue value, OirisHTTPHeader *headers, size_t *count,
                                   const char *operation) {
  *count = 0;
  if (value.kind == O_NULL) return 1;
  OirisHTTPHeaderCollector collector = {headers, 0, 0};
  if (value.kind == O_LIST) {
    for (size_t i = 0; i < value.as.list.len; i++) {
      OValue pair = value.as.list.items[i];
      if (pair.kind != O_LIST || pair.as.list.len != 2) {
        osiris_tls_http_error(operation, "", "headers must be a map or list of name/value pairs");
        return 0;
      }
      const char *name = NULL; size_t name_len = 0; const char *data = NULL; size_t length = 0;
      if (!osiris_tls_http_string(pair.as.list.items[0], &name, &name_len, operation) ||
          !osiris_tls_http_string(pair.as.list.items[1], &data, &length, operation) ||
          !osiris_tls_http_add_header(headers, &collector.count, name, name_len, data, length)) {
        osiris_tls_http_error(operation, "", "invalid or excessive HTTP header");
        return 0;
      }
    }
  } else if (osiris_http_map_reader) {
    if (!osiris_http_map_reader(osiris_tls_http_userdata, value,
                                osiris_tls_http_collect_header, &collector)) {
      osiris_tls_http_error(operation, "", "HTTP header map reader rejected the map");
      return 0;
    }
  } else {
    osiris_tls_http_error(operation, "", "native HTTP map reader is not installed");
    return 0;
  }
  if (collector.failed) {
    osiris_tls_http_error(operation, "", "invalid or excessive HTTP header");
    return 0;
  }
  *count = collector.count;
  return 1;
}

static int osiris_tls_http_url(OValue value, OirisHTTPURL *url, const char *operation) {
  const char *input = NULL; size_t length = 0;
  if (!osiris_tls_http_string(value, &input, &length, operation) || length == 0 || length >= sizeof(url->host)) return 0;
  memset(url, 0, sizeof(*url));
  const char *authority = NULL;
  if (length >= 7 && !memcmp(input, "http://", 7)) { url->secure = 0; authority = input + 7; }
  else if (length >= 8 && !memcmp(input, "https://", 8)) { url->secure = 1; authority = input + 8; }
  else { osiris_tls_http_error(operation, input, "HTTP URL must use http:// or https://"); return 0; }
  size_t authority_length = length - (size_t)(authority - input);
  const char *slash = memchr(authority, '/', authority_length);
  size_t host_length = slash ? (size_t)(slash - authority) : authority_length;
  if (host_length == 0 || host_length >= sizeof(url->host)) { osiris_tls_http_error(operation, input, "HTTP authority is too long"); return 0; }
  if (slash) {
    size_t target_length = authority_length - host_length;
    if (target_length >= sizeof(url->target)) { osiris_tls_http_error(operation, input, "HTTP target is too long"); return 0; }
    memcpy(url->target, slash, target_length); url->target[target_length] = 0;
  } else strcpy(url->target, "/");
  memcpy(url->authority, authority, host_length); url->authority[host_length] = 0;
  const char *host_start = authority;
  const char *port_start = NULL;
  if (host_length && authority[0] == '[') {
    const char *close = memchr(authority, ']', host_length);
    if (!close) { osiris_tls_http_error(operation, input, "invalid IPv6 HTTP authority"); return 0; }
    size_t host_copy = (size_t)(close - authority - 1);
    if (host_copy == 0 || host_copy >= sizeof(url->host)) return 0;
    memcpy(url->host, authority + 1, host_copy); url->host[host_copy] = 0;
    if ((size_t)(close - authority + 1) < host_length && close[1] == ':') port_start = close + 2;
  } else {
    const char *colon = memchr(authority, ':', host_length);
    size_t host_copy = colon ? (size_t)(colon - authority) : host_length;
    if (host_copy == 0 || host_copy >= sizeof(url->host)) return 0;
    memcpy(url->host, host_start, host_copy); url->host[host_copy] = 0;
    if (colon) port_start = colon + 1;
  }
  if (port_start) {
    size_t port_length = (size_t)(authority + host_length - port_start);
    if (port_length == 0 || port_length >= sizeof(url->port)) return 0;
    memcpy(url->port, port_start, port_length); url->port[port_length] = 0;
  } else strcpy(url->port, url->secure ? "443" : "80");
  return 1;
}

static int osiris_tls_http_provider_stream(OValue address, OValue server_name,
                                            OValue roots, int64_t timeout,
                                            int secure, OirisTLSHTTPStream *stream,
                                            const char *operation) {
  const char *address_data = NULL; size_t address_length = 0;
  if (!osiris_tls_http_string(address, &address_data, &address_length, operation)) return 0;
  char address_copy[OSIRIS_TLS_HTTP_MAX_URL];
  memcpy(address_copy, address_data, address_length); address_copy[address_length] = 0;
  if (secure) {
    if (!osiris_tls_provider || osiris_tls_provider->abi_version != 1 ||
        !(osiris_tls_provider->capabilities & OSIRIS_TLS_CAP_DIAL) || !osiris_tls_provider->dial) {
      osiris_tls_http_error(operation, address_copy, "TLS dial provider capability is not installed"); return 0;
    }
    const char *server_data = ""; size_t server_length = 0;
    if (server_name.kind != O_NULL && !osiris_tls_http_string(server_name, &server_data, &server_length, operation)) return 0;
    char server_copy[OSIRIS_TLS_HTTP_MAX_URL];
    if (server_length >= sizeof(server_copy)) return 0;
    memcpy(server_copy, server_data, server_length); server_copy[server_length] = 0;
    const char *roots_data = ""; size_t roots_length = 0;
    if (roots.kind != O_NULL && !osiris_tls_http_string(roots, &roots_data, &roots_length, operation)) return 0;
    char *roots_copy = (char *)malloc(roots_length + 1);
    if (!roots_copy) { osiris_tls_http_error(operation, address_copy, "out of memory"); return 0; }
    memcpy(roots_copy, roots_data, roots_length); roots_copy[roots_length] = 0;
    char error[256] = {0};
    int result = osiris_tls_provider->dial(osiris_tls_provider->userdata, address_copy,
                                           server_copy, roots_copy, timeout, stream,
                                           error, sizeof(error));
    free(roots_copy);
    if (result != 0) { osiris_tls_http_error(operation, address_copy, error[0] ? error : "TLS provider dial failed"); return 0; }
    return 1;
  }
  if (!osiris_http_transport_provider || osiris_http_transport_provider->abi_version != 1 ||
      !(osiris_http_transport_provider->capabilities & OSIRIS_HTTP_CAP_CLIENT) ||
      !osiris_http_transport_provider->dial) {
    osiris_tls_http_error(operation, address_copy, "HTTP transport provider capability is not installed"); return 0;
  }
  char error[256] = {0};
  if (osiris_http_transport_provider->dial(osiris_http_transport_provider->userdata,
                                           address_copy, timeout, stream, error, sizeof(error)) != 0) {
    osiris_tls_http_error(operation, address_copy, error[0] ? error : "HTTP transport dial failed"); return 0;
  }
  return 1;
}

static int osiris_tls_http_status(const char *line, int *status) {
  if (strncmp(line, "HTTP/1.", 7) != 0 || line[8] != ' ') return 0;
  char *end = NULL; long value = strtol(line + 9, &end, 10);
  if (end == line + 9 || value < 100 || value > 599) return 0;
  *status = (int)value; return 1;
}

static int osiris_tls_http_response_headers(OirisTLSHTTPResource *resource,
                                             int64_t timeout, int *status,
                                             OirisHTTPHeader *headers, size_t *count,
                                             int *chunked, int64_t *content_length,
                                             const char *operation) {
  char line[OSIRIS_TLS_HTTP_MAX_LINE];
  if (!osiris_tls_http_line(resource, line, sizeof(line), timeout, operation) ||
      !osiris_tls_http_status(line, status)) {
    osiris_tls_http_error(operation, "", "invalid HTTP status line"); return 0;
  }
  *count = 0; *chunked = 0; *content_length = -1;
  while (1) {
    if (!osiris_tls_http_line(resource, line, sizeof(line), timeout, operation)) return 0;
    if (!line[0]) break;
    char *colon = strchr(line, ':');
    if (!colon) { osiris_tls_http_error(operation, "", "invalid HTTP header"); return 0; }
    size_t name_length = (size_t)(colon - line); const char *value = colon + 1;
    while (*value == ' ' || *value == '\t') value++;
    if (!osiris_tls_http_add_header(headers, count, line, name_length, value, strlen(value))) {
      osiris_tls_http_error(operation, "", "invalid or excessive HTTP response headers"); return 0;
    }
		if (osiris_tls_http_case_equal_n(line, name_length, "Content-Length")) {
      char *end = NULL; long long parsed = strtoll(value, &end, 10);
	      while (end && (*end == ' ' || *end == '\t')) end++;
	      if (end == value || (end && *end) || parsed < 0 || parsed > OSIRIS_TLS_HTTP_MAX_BODY) {
        osiris_tls_http_error(operation, "", "invalid or excessive Content-Length"); return 0;
      }
      *content_length = (int64_t)parsed;
		} else if (osiris_tls_http_case_equal_n(line, name_length, "Transfer-Encoding") && strstr(value, "chunked")) {
      *chunked = 1;
    }
  }
  return 1;
}

static const char *osiris_tls_http_find_header(const OirisHTTPHeader *headers,
                                               size_t count, const char *name) {
  for (size_t i = 0; i < count; i++) {
    if (osiris_tls_http_case_equal(headers[i].name, name)) return headers[i].value;
  }
  return NULL;
}

static int osiris_tls_http_redirect_status(int status) {
  return status == 301 || status == 302 || status == 303 ||
         status == 307 || status == 308;
}

static void osiris_tls_http_remove_header(OirisHTTPHeader *headers, size_t *count,
                                          const char *name) {
  size_t write = 0;
  for (size_t read = 0; read < *count; read++) {
    if (osiris_tls_http_case_equal(headers[read].name, name)) {
      free(headers[read].name);
      free(headers[read].value);
      continue;
    }
    if (write != read) headers[write] = headers[read];
    write++;
  }
  *count = write;
}

static int osiris_tls_http_same_authority(const OirisHTTPURL *left,
                                          const OirisHTTPURL *right) {
  return left->secure == right->secure &&
         osiris_tls_http_case_equal(left->authority, right->authority);
}

static int osiris_tls_http_redirect_url(const OirisHTTPURL *base,
                                        const char *location, char *output,
                                        size_t capacity, const char *operation) {
  if (!location || !location[0]) {
    osiris_tls_http_error(operation, "", "redirect response has an empty Location");
    return 0;
  }
  size_t location_length = strlen(location);
  if (location_length >= OSIRIS_TLS_HTTP_MAX_URL) {
    osiris_tls_http_error(operation, "", "redirect Location exceeds the URL limit");
    return 0;
  }
  char target[OSIRIS_TLS_HTTP_MAX_URL];
  memcpy(target, location, location_length + 1);
  char *fragment = strchr(target, '#');
  if (fragment) *fragment = 0;
  if (!target[0]) {
    osiris_tls_http_error(operation, "", "redirect Location has no request target");
    return 0;
  }
  const char *scheme = base->secure ? "https" : "http";
  int written;
  if (!strncmp(target, "http://", 7) || !strncmp(target, "https://", 8)) {
    written = snprintf(output, capacity, "%s", target);
  } else if (!strncmp(target, "//", 2)) {
    written = snprintf(output, capacity, "%s:%s", scheme, target);
  } else if (target[0] == '/') {
    written = snprintf(output, capacity, "%s://%s%s", scheme, base->authority, target);
  } else {
    const char *query = strchr(base->target, '?');
    size_t path_length = query ? (size_t)(query - base->target) : strlen(base->target);
    const char *last_slash = NULL;
    for (size_t i = 0; i < path_length; i++) {
      if (base->target[i] == '/') last_slash = base->target + i;
    }
    size_t directory_length = last_slash ? (size_t)(last_slash - base->target + 1) : 1;
    if (target[0] == '?') directory_length = path_length;
    written = snprintf(output, capacity, "%s://%s%.*s%s", scheme,
                       base->authority, (int)directory_length, base->target,
                       target);
  }
  if (written < 0 || (size_t)written >= capacity) {
    osiris_tls_http_error(operation, "", "redirect URL exceeds the URL limit");
    return 0;
  }
  OirisHTTPURL validated;
  if (!osiris_tls_http_url(ostring(output), &validated, operation)) return 0;
  return 1;
}

static int osiris_tls_http_handler_error(OValue value, char *error,
                                         size_t error_capacity) {
  if (value.kind == O_ERROR && value.as.error.message.data) {
    osiris_tls_http_copy_error(error, error_capacity, value.as.error.message.data);
    return 1;
  }
  if (ohaderror && oerrormessage) {
    osiris_tls_http_copy_error(error, error_capacity, oerrormessage);
    return 1;
  }
  return 0;
}

static int osiris_tls_http_invoke_handler(OValue handler, OValue request,
                                          OValue *response, char *error,
                                          size_t error_capacity) {
  if (osiris_http_handler) {
    if (osiris_http_handler(osiris_tls_http_userdata, request, response,
                            error, error_capacity) != 0) return 0;
    return 1;
  }
  if (handler.kind != O_CLOSURE) {
    osiris_tls_http_copy_error(error, error_capacity,
                               "HTTP handler is not callable");
    return 0;
  }
  OValue argument = request;
  OValue value = ocall(handler, &argument, 1);
  if (value.kind == O_NULL && ohaderror) {
    osiris_tls_http_handler_error(value, error, error_capacity);
    return 0;
  }
  if (value.kind == O_ERROR) {
    osiris_tls_http_handler_error(value, error, error_capacity);
    return 0;
  }
  *response = value;
  return 1;
}

static int osiris_tls_http_read_body(OirisTLSHTTPResource *resource, int64_t timeout,
                                     OirisHTTPHeader *headers, size_t count,
                                     int chunked, int64_t content_length,
                                     unsigned char **body, size_t *body_length,
                                     const char *operation) {
  (void)headers; (void)count;
  size_t capacity = 4096; size_t length = 0;
  unsigned char *data = (unsigned char *)malloc(capacity);
  if (!data) { osiris_tls_http_error(operation, "", "out of memory"); return 0; }
  int done = 0;
  while (!done) {
    int64_t wanted = chunked ? 0 : content_length >= 0 ? content_length - (int64_t)length : 4096;
    if (chunked) {
      char line[64]; if (!osiris_tls_http_line(resource, line, sizeof(line), timeout, operation)) { free(data); return 0; }
      char *end = NULL; unsigned long long chunk = strtoull(line, &end, 16);
      if (end == line || (*end && *end != ';') || chunk > OSIRIS_TLS_HTTP_MAX_BODY) { free(data); osiris_tls_http_error(operation, "", "invalid HTTP chunk size"); return 0; }
      if (chunk == 0) {
        do { if (!osiris_tls_http_line(resource, line, sizeof(line), timeout, operation)) { free(data); return 0; } } while (line[0]);
        done = 1; continue;
      }
      wanted = (int64_t)chunk;
    }
    if (!chunked && content_length >= 0 && wanted <= 0) { done = 1; continue; }
    if (wanted > (int64_t)(OSIRIS_TLS_HTTP_MAX_BODY - length)) { free(data); osiris_tls_http_error(operation, "", "HTTP response body exceeds 8 MiB"); return 0; }
    while (wanted > 0) {
      if (length == capacity) { size_t next = capacity * 2; if (next > OSIRIS_TLS_HTTP_MAX_BODY) next = OSIRIS_TLS_HTTP_MAX_BODY; unsigned char *grown = (unsigned char *)realloc(data, next); if (!grown) { free(data); osiris_tls_http_error(operation, "", "out of memory"); return 0; } data = grown; capacity = next; }
      size_t request = capacity - length; if ((int64_t)request > wanted) request = (size_t)wanted;
      if (resource->buffer_offset < resource->buffered) {
        size_t available = resource->buffered - resource->buffer_offset; if (available > request) available = request;
        memcpy(data + length, resource->buffer + resource->buffer_offset, available); resource->buffer_offset += available; length += available; wanted -= (int64_t)available;
      } else {
        int64_t got = osiris_tls_http_read_raw(resource, data + length, request, timeout, operation);
        if (got == 0) { if (!chunked && content_length < 0) { done = 1; break; } free(data); osiris_tls_http_error(operation, "", "unexpected end of HTTP body"); return 0; }
        if (got < 0) { free(data); return 0; }
        length += (size_t)got; wanted -= got;
      }
    }
    if (chunked) { unsigned char crlf[2]; if (!osiris_tls_http_get_byte(resource, &crlf[0], timeout, operation) || !osiris_tls_http_get_byte(resource, &crlf[1], timeout, operation) || crlf[0] != '\r' || crlf[1] != '\n') { free(data); osiris_tls_http_error(operation, "", "invalid HTTP chunk terminator"); return 0; } }
    if (!chunked && content_length >= 0) done = 1;
    if (!chunked && content_length < 0 && length == OSIRIS_TLS_HTTP_MAX_BODY) done = 1;
  }
  *body = data; *body_length = length; return 1;
}

static OValue osiris_tls_http_make_response(int status, const char *final_url,
                                             OirisHTTPHeader *headers, size_t header_count,
                                             const unsigned char *body, size_t body_length,
                                             OValue body_stream, int include_stream) {
  if (!osiris_http_map_constructor) return osiris_tls_http_error("http_response", final_url, "native HTTP map constructor is not installed");
  OValue *header_pairs = header_count ? (OValue *)calloc(header_count, sizeof(OValue)) : NULL;
  if (header_count && !header_pairs) return osiris_tls_http_error("http_response", final_url, "out of memory");
  for (size_t i = 0; i < header_count; i++) { OValue pair[2] = {ostring(headers[i].name), ostring(headers[i].value)}; header_pairs[i] = olist(pair, 2); }
  OValue header_value = olist(header_pairs, header_count); free(header_pairs);
  OValue entries[10]; size_t count = 0;
  entries[count++] = (OValue){O_STRING,0,{.s={"status",6}}}; entries[count++] = oint(status);
  entries[count++] = (OValue){O_STRING,0,{.s={"headers",7}}}; entries[count++] = header_value;
  entries[count++] = (OValue){O_STRING,0,{.s={"body",4}}}; OValue body_value = osiris_tls_http_owned_string(body ? body : (const unsigned char *)"", body_length, "http_response"); if (body_value.kind == O_NULL) return body_value; entries[count++] = body_value;
  if (include_stream) { OValue stream_entry[2] = {(OValue){O_STRING,0,{.s={"body_stream",11}}}, body_stream}; entries[count++] = stream_entry[0]; entries[count++] = stream_entry[1]; }
  entries[count++] = (OValue){O_STRING,0,{.s={"url",3}}}; entries[count++] = ostring(final_url);
  OirisHTTPMapEntry map_entries[8];
  size_t map_count = 0;
  map_entries[map_count++] = (OirisHTTPMapEntry){"status", entries[1]};
  map_entries[map_count++] = (OirisHTTPMapEntry){"headers", entries[3]};
  map_entries[map_count++] = (OirisHTTPMapEntry){"body", body_value};
  if (include_stream) map_entries[map_count++] = (OirisHTTPMapEntry){"body_stream", body_stream};
  OValue url_value = osiris_tls_http_owned_string((const unsigned char *)final_url, strlen(final_url), "http_response"); if (url_value.kind == O_NULL) return url_value;
  map_entries[map_count++] = (OirisHTTPMapEntry){"url", url_value};
  return osiris_http_map_constructor(osiris_tls_http_userdata, map_entries, map_count);
}

static int osiris_tls_http_request_stream(OValue method, OValue url, OValue headers_value,
                                          OValue body_value, OValue timeout_value,
                                          int streaming, OValue *result) {
  const char *method_data = NULL; size_t method_length = 0;
  if (!osiris_tls_http_string(method, &method_data, &method_length, "http_request") || method_length == 0 || method_length > 32) return 0;
  char method_copy[33]; memcpy(method_copy, method_data, method_length); method_copy[method_length] = 0;
  const char *url_data = NULL; size_t url_length = 0;
  if (!osiris_tls_http_string(url, &url_data, &url_length, "http_request")) return 0;
  char final_url[OSIRIS_TLS_HTTP_MAX_URL]; memcpy(final_url, url_data, url_length); final_url[url_length] = 0;
  OirisHTTPHeader headers[OSIRIS_TLS_HTTP_MAX_HEADERS]; memset(headers, 0, sizeof(headers)); size_t header_count = 0;
  if (!osiris_tls_http_headers(headers_value, headers, &header_count, "http_request")) return 0;
  const unsigned char *body_data = (const unsigned char *)""; size_t body_length = 0; unsigned char *owned_body = NULL;
  if (body_value.kind != O_NULL) {
    if (body_value.kind == O_STRING) { body_data = (const unsigned char *)body_value.as.s.data; body_length = body_value.as.s.len; }
    else if (body_value.kind == O_LIST) {
      body_length = body_value.as.list.len; if (body_length > OSIRIS_TLS_HTTP_MAX_BODY) { osiris_tls_http_free_headers(headers, header_count); osiris_tls_http_error("http_request", final_url, "HTTP request body exceeds 8 MiB"); return 0; }
      owned_body = (unsigned char *)malloc(body_length ? body_length : 1); if (!owned_body && body_length) { osiris_tls_http_free_headers(headers, header_count); osiris_tls_http_error("http_request", final_url, "out of memory"); return 0; }
      for (size_t i = 0; i < body_length; i++) { if (body_value.as.list.items[i].kind != O_UINT || body_value.as.list.items[i].as.u > 255) { free(owned_body); osiris_tls_http_free_headers(headers, header_count); osiris_tls_http_error("http_request", final_url, "HTTP body byte list contains a non-byte"); return 0; } owned_body[i] = (unsigned char)body_value.as.list.items[i].as.u; }
      body_data = owned_body;
    } else { osiris_tls_http_free_headers(headers, header_count); osiris_tls_http_error("http_request", final_url, "HTTP body must be a string, byte list, or null"); return 0; }
  }
  if (body_length > OSIRIS_TLS_HTTP_MAX_BODY) { free(owned_body); osiris_tls_http_free_headers(headers, header_count); osiris_tls_http_error("http_request", final_url, "HTTP request body exceeds 8 MiB"); return 0; }
  int64_t timeout = 0; if (!osiris_tls_http_timeout(timeout_value, &timeout, "http_request")) { free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0; }
  size_t redirect_count = 0;
  for (;;) {
    OirisHTTPURL parsed;
    if (!osiris_tls_http_url(ostring(final_url), &parsed, "http_request")) {
      free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
    }
    if (osiris_tls_http_cancelled()) {
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_cancel_error("http_request"); return 0;
    }
    char address[OSIRIS_TLS_HTTP_MAX_URL];
    int address_written = snprintf(address, sizeof(address), "%s:%s", parsed.host, parsed.port);
    if (address_written < 0 || (size_t)address_written >= sizeof(address)) {
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_error("http_request", parsed.authority, "HTTP address exceeds the URL limit"); return 0;
    }
    OValue server_name = parsed.secure ? ostring(parsed.host) : onull();
    OValue roots = onull(); OirisTLSHTTPStream stream = {0};
    if (!osiris_tls_http_provider_stream(ostring(address), server_name, roots, timeout,
                                         parsed.secure, &stream, "http_request")) {
      free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
    }
    OirisTLSHTTPResource resource = {0}; resource.magic = OSIRIS_TLS_HTTP_MAGIC;
    resource.kind = OSIRIS_TLS_HTTP_RESOURCE_STREAM; resource.stream = stream;
    size_t request_capacity = strlen(method_copy) + strlen(parsed.target) +
                              strlen(parsed.authority) + body_length + 512;
    if (request_capacity > OSIRIS_TLS_HTTP_MAX_REQUEST) {
      osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_error("http_request", parsed.authority, "HTTP request exceeds the bounded request limit"); return 0;
    }
    char *request = (char *)malloc(request_capacity);
    if (!request) {
      osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_error("http_request", parsed.authority, "out of memory"); return 0;
    }
    int written = snprintf(request, request_capacity,
                           "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\nContent-Length: %zu\r\n",
                           method_copy, parsed.target, parsed.authority, body_length);
    if (written < 0 || (size_t)written >= request_capacity) {
      free(request); osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_error("http_request", parsed.authority, "HTTP request headers are too large"); return 0;
    }
    size_t request_length = (size_t)written;
    for (size_t i = 0; i < header_count; i++) {
      int part = snprintf(request + request_length, request_capacity - request_length,
                          "%s: %s\r\n", headers[i].name, headers[i].value);
      if (part < 0 || (size_t)part >= request_capacity - request_length) {
        free(request); osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
        free(owned_body); osiris_tls_http_free_headers(headers, header_count);
        osiris_tls_http_error("http_request", parsed.authority, "HTTP request headers are too large"); return 0;
      }
      request_length += (size_t)part;
    }
    if (request_length + 2 + body_length >= request_capacity) {
      free(request); osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      osiris_tls_http_error("http_request", parsed.authority, "HTTP request exceeds the bounded request limit"); return 0;
    }
    memcpy(request + request_length, "\r\n", 2); request_length += 2;
    int okay = osiris_tls_http_write_all(&resource, (unsigned char *)request,
                                         request_length, timeout, "http_request") &&
               osiris_tls_http_write_all(&resource, body_data, body_length,
                                         timeout, "http_request");
    free(request);
    if (!okay) {
      osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
    }
    int status = 0, chunked = 0; int64_t content_length = -1;
    OirisHTTPHeader response_headers[OSIRIS_TLS_HTTP_MAX_HEADERS];
    memset(response_headers, 0, sizeof(response_headers)); size_t response_count = 0;
    if (!osiris_tls_http_response_headers(&resource, timeout, &status,
                                          response_headers, &response_count,
                                          &chunked, &content_length, "http_request")) {
      osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      osiris_tls_http_free_headers(response_headers, response_count);
      free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
    }
    const char *location = osiris_tls_http_find_header(response_headers,
                                                       response_count, "Location");
    if (osiris_tls_http_redirect_status(status) && location) {
      if (redirect_count >= OSIRIS_TLS_HTTP_MAX_REDIRECTS) {
        osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
        osiris_tls_http_free_headers(response_headers, response_count);
        free(owned_body); osiris_tls_http_free_headers(headers, header_count);
        osiris_tls_http_error("http_request", final_url, "HTTP redirect limit exceeded"); return 0;
      }
      char next_url[OSIRIS_TLS_HTTP_MAX_URL];
      if (!osiris_tls_http_redirect_url(&parsed, location, next_url,
                                        sizeof(next_url), "http_request")) {
        osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
        osiris_tls_http_free_headers(response_headers, response_count);
        free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
      }
      unsigned char *discarded_body = NULL; size_t discarded_length = 0;
      int drained = osiris_tls_http_read_body(&resource, timeout,
                                              response_headers, response_count,
                                              chunked, content_length,
                                              &discarded_body, &discarded_length,
                                              "http_request");
      free(discarded_body);
      OValue close_result = osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
      osiris_tls_http_free_headers(response_headers, response_count);
      if (!drained || close_result.kind == O_NULL) {
        free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
      }
      OirisHTTPURL next_parsed;
      if (!osiris_tls_http_url(ostring(next_url), &next_parsed, "http_request")) {
        free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
      }
      int switch_to_get = (status == 301 || status == 302 || status == 303) &&
                          strcmp(method_copy, "GET") != 0 && strcmp(method_copy, "HEAD") != 0;
      if (switch_to_get) {
        strcpy(method_copy, "GET"); body_data = (const unsigned char *)""; body_length = 0;
        osiris_tls_http_remove_header(headers, &header_count, "Content-Length");
        osiris_tls_http_remove_header(headers, &header_count, "Content-Type");
        osiris_tls_http_remove_header(headers, &header_count, "Transfer-Encoding");
      }
      if (!osiris_tls_http_same_authority(&parsed, &next_parsed)) {
        osiris_tls_http_remove_header(headers, &header_count, "Authorization");
        osiris_tls_http_remove_header(headers, &header_count, "Proxy-Authorization");
        osiris_tls_http_remove_header(headers, &header_count, "Cookie");
      }
      memcpy(final_url, next_url, strlen(next_url) + 1);
      redirect_count++;
      continue;
    }
    if (streaming) {
      OirisTLSHTTPResource *body_resource = (OirisTLSHTTPResource *)calloc(1, sizeof(*body_resource));
      if (!body_resource || !otrack(body_resource)) {
        free(body_resource); osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
        osiris_tls_http_free_headers(response_headers, response_count);
        free(owned_body); osiris_tls_http_free_headers(headers, header_count);
        osiris_tls_http_error("http_request_stream", parsed.authority, "out of memory"); return 0;
      }
      *body_resource = resource; body_resource->kind = OSIRIS_TLS_HTTP_RESOURCE_BODY;
      body_resource->chunked = chunked; body_resource->remaining = content_length;
      body_resource->chunk_remaining = 0; body_resource->chunk_done = 0;
      OValue body_stream = osiris_tls_http_resource_value(body_resource);
      *result = osiris_tls_http_make_response(status, final_url, response_headers,
                                              response_count, NULL, 0, body_stream, 1);
      if (result->kind == O_NULL) osiris_tls_http_close(body_stream);
      osiris_tls_http_free_headers(response_headers, response_count);
      free(owned_body); osiris_tls_http_free_headers(headers, header_count);
      return result->kind != O_NULL;
    }
    unsigned char *response_body = NULL; size_t response_body_length = 0;
    int okay_body = osiris_tls_http_read_body(&resource, timeout, response_headers,
                                              response_count, chunked, content_length,
                                              &response_body, &response_body_length,
                                              "http_request");
    OValue close_result = osiris_tls_http_close(osiris_tls_http_resource_value(&resource));
    if (!okay_body || close_result.kind == O_NULL) {
      free(response_body); osiris_tls_http_free_headers(response_headers, response_count);
      free(owned_body); osiris_tls_http_free_headers(headers, header_count); return 0;
    }
    *result = osiris_tls_http_make_response(status, final_url, response_headers,
                                            response_count, response_body,
                                            response_body_length, onull(), 0);
    free(response_body); osiris_tls_http_free_headers(response_headers, response_count);
    free(owned_body); osiris_tls_http_free_headers(headers, header_count);
    return result->kind != O_NULL;
  }
}

static void osiris_tls_http_ensure_provider(void) {
#ifdef OSIRIS_OPENSSL_PROVIDER
  if (!osiris_tls_provider) osiris_tls_provider = osiris_native_openssl_provider();
#endif
}

static OValue osiris_native_tls_dial(OValue address, OValue server_name, OValue roots, OValue timeout) {
  osiris_tls_http_ensure_provider();
  int64_t timeout_ms = 0; if (!osiris_tls_http_timeout(timeout, &timeout_ms, "tls_dial")) return onull();
  OirisTLSHTTPStream stream = {0}; if (!osiris_tls_http_provider_stream(address, server_name, roots, timeout_ms, 1, &stream, "tls_dial")) return onull();
  OirisTLSHTTPResource *resource = osiris_tls_http_new(OSIRIS_TLS_HTTP_RESOURCE_STREAM, stream, "tls_dial"); return resource ? osiris_tls_http_resource_value(resource) : onull();
}

static OValue osiris_native_tls_listen(OValue address, OValue certificate, OValue key) {
  osiris_tls_http_ensure_provider();
  const char *address_data = NULL; size_t address_length = 0; const char *certificate_data = NULL; size_t certificate_length = 0; const char *key_data = NULL; size_t key_length = 0;
  if (!osiris_tls_http_string(address, &address_data, &address_length, "tls_listen") || !osiris_tls_http_string(certificate, &certificate_data, &certificate_length, "tls_listen") || !osiris_tls_http_string(key, &key_data, &key_length, "tls_listen")) return onull();
  char address_copy[OSIRIS_TLS_HTTP_MAX_URL], certificate_copy[OSIRIS_TLS_HTTP_MAX_URL], key_copy[OSIRIS_TLS_HTTP_MAX_URL];
  if (address_length >= sizeof(address_copy) || certificate_length >= sizeof(certificate_copy) || key_length >= sizeof(key_copy)) return osiris_tls_http_error("tls_listen", "", "TLS listen argument is too long");
  memcpy(address_copy, address_data, address_length); address_copy[address_length] = 0; memcpy(certificate_copy, certificate_data, certificate_length); certificate_copy[certificate_length] = 0; memcpy(key_copy, key_data, key_length); key_copy[key_length] = 0;
  if (!osiris_tls_provider || osiris_tls_provider->abi_version != 1 || !(osiris_tls_provider->capabilities & OSIRIS_TLS_CAP_LISTEN) || !osiris_tls_provider->listen) return osiris_tls_http_error("tls_listen", address_copy, "TLS listen provider capability is not installed");
  OirisTLSHTTPStream listener = {0}; char error[256] = {0};
  if (osiris_tls_provider->listen(osiris_tls_provider->userdata, address_copy, certificate_copy, key_copy, &listener, error, sizeof(error)) != 0) return osiris_tls_http_error("tls_listen", address_copy, error[0] ? error : "TLS provider listen failed");
  OirisTLSHTTPResource *resource = osiris_tls_http_new(OSIRIS_TLS_HTTP_RESOURCE_LISTENER, listener, "tls_listen"); if (resource) resource->secure_listener = 1; return resource ? osiris_tls_http_resource_value(resource) : onull();
}

static OValue osiris_native_http_request(OValue method, OValue url, OValue headers, OValue body, OValue timeout) {
  OValue result = onull(); osiris_tls_http_request_stream(method, url, headers, body, timeout, 0, &result); return result;
}

static OValue osiris_native_http_get(OValue url, OValue timeout) {
  return osiris_native_http_request(ostring("GET"), url, onull(), onull(), timeout);
}

static OValue osiris_native_http_request_stream_value(OValue method, OValue url, OValue headers, OValue body, OValue timeout) {
  OValue result = onull(); osiris_tls_http_request_stream(method, url, headers, body, timeout, 1, &result); return result;
}

static OValue osiris_native_http_body_read(OValue body_value, OValue size_value, OValue timeout_value) {
  OirisTLSHTTPResource *body = osiris_tls_http_resource(body_value, OSIRIS_TLS_HTTP_RESOURCE_BODY, "http_body_read"); if (!body) return onull();
  int64_t size = 0, timeout = 0; if (!osiris_tls_http_timeout(timeout_value, &timeout, "http_body_read") || size_value.kind != O_UINT || size_value.as.u > OSIRIS_TLS_HTTP_MAX_BODY) return osiris_tls_http_error("http_body_read", "", "read size must be a bounded non-negative integer"); size = (int64_t)size_value.as.u;
  unsigned char *data = size ? (unsigned char *)malloc((size_t)size) : NULL; if (size && !data) return osiris_tls_http_error("http_body_read", "", "out of memory"); size_t length = 0;
  while (length < (size_t)size) { if (body->chunked && body->chunk_remaining == 0 && !body->chunk_done) { char line[64]; if (!osiris_tls_http_line(body, line, sizeof(line), timeout, "http_body_read")) { free(data); return onull(); } char *end = NULL; unsigned long long chunk = strtoull(line, &end, 16); if (end == line || (*end && *end != ';') || chunk > OSIRIS_TLS_HTTP_MAX_BODY) { free(data); return osiris_tls_http_error("http_body_read", "", "invalid HTTP chunk size"); } if (chunk == 0) { do { if (!osiris_tls_http_line(body, line, sizeof(line), timeout, "http_body_read")) { free(data); return onull(); } } while (line[0]); body->chunk_done = 1; break; } body->chunk_remaining = (int64_t)chunk; }
    if (body->remaining == 0 && !body->chunked) break;
    size_t wanted = (size_t)size - length; if (body->chunked && (int64_t)wanted > body->chunk_remaining) wanted = (size_t)body->chunk_remaining; if (!body->chunked && body->remaining > 0 && (int64_t)wanted > body->remaining) wanted = (size_t)body->remaining;
    int64_t got = osiris_tls_http_read_raw(body, data + length, wanted, timeout, "http_body_read"); if (got <= 0) break; length += (size_t)got; if (body->chunked) body->chunk_remaining -= got; if (!body->chunked && body->remaining > 0) body->remaining -= got; if (body->chunked && body->chunk_remaining == 0) { unsigned char crlf[2]; if (!osiris_tls_http_get_byte(body, &crlf[0], timeout, "http_body_read") || !osiris_tls_http_get_byte(body, &crlf[1], timeout, "http_body_read") || crlf[0] != '\r' || crlf[1] != '\n') { free(data); return osiris_tls_http_error("http_body_read", "", "invalid HTTP chunk terminator"); } }
  }
  OValue *items = length ? (OValue *)calloc(length, sizeof(OValue)) : NULL; if (length && !items) { free(data); return osiris_tls_http_error("http_body_read", "", "out of memory"); } for (size_t i = 0; i < length; i++) items[i] = ouintbits(data[i], 8); OValue result = olist(items, length); free(items); free(data); return result;
}

static OValue osiris_native_tls_stream_readchunk(OValue stream_value, OValue size_value, OValue timeout_value) {
  OirisTLSHTTPResource *stream = osiris_tls_http_resource(stream_value, OSIRIS_TLS_HTTP_RESOURCE_STREAM, "readchunk"); if (!stream) return onull();
  int64_t size = 0, timeout = 0; if (!osiris_tls_http_timeout(timeout_value, &timeout, "readchunk") || size_value.kind != O_UINT || size_value.as.u > OSIRIS_TLS_HTTP_MAX_BODY) return osiris_tls_http_error("readchunk", "", "read size must be a bounded non-negative integer"); size = (int64_t)size_value.as.u;
  unsigned char *data = size ? (unsigned char *)malloc((size_t)size) : NULL; if (size && !data) return osiris_tls_http_error("readchunk", "", "out of memory"); int64_t count = size ? osiris_tls_http_read_raw(stream, data, (size_t)size, timeout, "readchunk") : 0; if (count < 0) { free(data); return onull(); }
  OValue *items = count ? (OValue *)calloc((size_t)count, sizeof(OValue)) : NULL; if (count && !items) { free(data); return osiris_tls_http_error("readchunk", "", "out of memory"); } for (int64_t i = 0; i < count; i++) items[i] = ouintbits(data[i], 8); OValue result = olist(items, (size_t)count); free(items); free(data); return result;
}

static OValue osiris_native_tls_stream_writechunk(OValue stream_value, OValue data_value, OValue timeout_value) {
  OirisTLSHTTPResource *stream = osiris_tls_http_resource(stream_value, OSIRIS_TLS_HTTP_RESOURCE_STREAM, "writechunk"); if (!stream) return onull(); int64_t timeout = 0; if (!osiris_tls_http_timeout(timeout_value, &timeout, "writechunk")) return onull();
  const unsigned char *data = NULL; size_t length = 0; unsigned char *owned = NULL; if (data_value.kind == O_STRING) { data = (const unsigned char *)data_value.as.s.data; length = data_value.as.s.len; } else if (data_value.kind == O_LIST && data_value.as.list.len <= OSIRIS_TLS_HTTP_MAX_BODY) { length = data_value.as.list.len; owned = length ? (unsigned char *)malloc(length) : NULL; if (length && !owned) return osiris_tls_http_error("writechunk", "", "out of memory"); for (size_t i = 0; i < length; i++) { if (data_value.as.list.items[i].kind != O_UINT || data_value.as.list.items[i].as.u > 255) { free(owned); return osiris_tls_http_error("writechunk", "", "write data must be bytes"); } owned[i] = (unsigned char)data_value.as.list.items[i].as.u; } data = owned; } else return osiris_tls_http_error("writechunk", "", "write data must be a string or byte list");
  int okay = osiris_tls_http_write_all(stream, data, length, timeout, "writechunk"); free(owned); return okay ? ouintbits((uint64_t)length, 64) : onull();
}

static int osiris_native_http_serve(OValue listener_value, OValue body_or_handler, OValue status_value) {
  OirisTLSHTTPResource *listener = osiris_tls_http_resource(listener_value, OSIRIS_TLS_HTTP_RESOURCE_LISTENER, "http_serve"); if (!listener) return -1;
  int64_t timeout = 0; int status = 200; if (status_value.kind != O_NULL) { if (status_value.kind == O_UINT && status_value.as.u >= 100 && status_value.as.u <= 599) status = (int)status_value.as.u; else if (status_value.kind == O_INT && status_value.as.i >= 100 && status_value.as.i <= 599) status = (int)status_value.as.i; else { osiris_tls_http_error("http_serve", "", "status must be an integer between 100 and 599"); return -1; } }
  if (listener->secure_listener) { if (!osiris_tls_provider || osiris_tls_provider->abi_version != 1 || !(osiris_tls_provider->capabilities & OSIRIS_TLS_CAP_LISTEN) || !osiris_tls_provider->accept) return (int)(osiris_tls_http_error("http_serve", "", "TLS server provider capability is not installed"), -1); } else if (!osiris_http_transport_provider || osiris_http_transport_provider->abi_version != 1 || !(osiris_http_transport_provider->capabilities & OSIRIS_HTTP_CAP_SERVER) || !osiris_http_transport_provider->accept) return (int)(osiris_tls_http_error("http_serve", "", "HTTP server provider capability is not installed"), -1);
  if (body_or_handler.kind == O_CLOSURE && !osiris_http_handler && !body_or_handler.as.closure.call) return (int)(osiris_tls_http_error("http_serve", "", "native HTTP closure handler is not callable"), -1);
  const char *static_body = NULL; size_t static_length = 0; if (body_or_handler.kind != O_CLOSURE && !osiris_tls_http_string(body_or_handler, &static_body, &static_length, "http_serve")) return -1; if (static_length > OSIRIS_TLS_HTTP_MAX_BODY) return (int)(osiris_tls_http_error("http_serve", "", "HTTP response body exceeds 8 MiB"), -1);
  for (;;) {
    if (osiris_tls_http_cancelled()) return (osiris_tls_http_cancel_error("http_serve"), -1);
    OirisTLSHTTPStream accepted = {0}; char error[256] = {0}; if (listener->secure_listener) { if (!osiris_tls_provider || !osiris_tls_provider->accept || !(osiris_tls_provider->capabilities & OSIRIS_TLS_CAP_LISTEN) || osiris_tls_provider->accept(osiris_tls_provider->userdata, &listener->stream, timeout, &accepted, error, sizeof(error)) != 0) return (int)(osiris_tls_http_error("http_serve", "", error[0] ? error : "TLS accept failed"), -1); } else if (osiris_http_transport_provider->accept(osiris_http_transport_provider->userdata, &listener->stream, timeout, &accepted, error, sizeof(error)) != 0) return (int)(osiris_tls_http_error("http_serve", "", error[0] ? error : "HTTP accept failed"), -1);
    OirisTLSHTTPResource connection = {0}; connection.magic = OSIRIS_TLS_HTTP_MAGIC; connection.kind = OSIRIS_TLS_HTTP_RESOURCE_STREAM; connection.stream = accepted;
    char line[OSIRIS_TLS_HTTP_MAX_LINE]; if (!osiris_tls_http_line(&connection, line, sizeof(line), timeout, "http_serve")) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); return -1; }
    char *space = strchr(line, ' '); if (!space) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_error("http_serve", "", "invalid HTTP request line"); return -1; }
    *space = 0; char request_method[33]; size_t request_method_length = strlen(line); if (request_method_length == 0 || request_method_length >= sizeof(request_method)) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_error("http_serve", "", "HTTP method is too long"); return -1; } memcpy(request_method, line, request_method_length + 1);
    char *target = space + 1; char *version = strchr(target, ' '); if (!version || strcmp(version + 1, "HTTP/1.1") != 0) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_error("http_serve", "", "unsupported HTTP request version"); return -1; }
    *version = 0; char request_target[OSIRIS_TLS_HTTP_MAX_URL]; if (strlen(target) >= sizeof(request_target)) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_error("http_serve", "", "HTTP target is too long"); return -1; } strcpy(request_target, target);
    OirisHTTPHeader request_headers[OSIRIS_TLS_HTTP_MAX_HEADERS]; memset(request_headers, 0, sizeof(request_headers)); size_t request_count = 0; int request_chunked = 0; int64_t request_length = -1; while (1) { if (!osiris_tls_http_line(&connection, line, sizeof(line), timeout, "http_serve")) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); return -1; } if (!line[0]) break; char *colon = strchr(line, ':'); size_t name_length = colon ? (size_t)(colon-line) : 0; if (!colon || !osiris_tls_http_add_header(request_headers, &request_count, line, name_length, colon+1, strlen(colon+1))) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_error("http_serve", "", "invalid or excessive HTTP request headers"); return -1; } if (osiris_tls_http_case_equal_n(line, name_length, "Content-Length")) { char *end = NULL; long long parsed = strtoll(colon + 1, &end, 10); while (end && (*end == ' ' || *end == '\t')) end++; if (end == colon + 1 || (end && *end) || parsed < 0 || parsed > OSIRIS_TLS_HTTP_MAX_BODY) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_error("http_serve", "", "invalid request Content-Length"); return -1; } request_length = (int64_t)parsed; } else if (osiris_tls_http_case_equal_n(line, name_length, "Transfer-Encoding") && strstr(colon + 1, "chunked")) request_chunked = 1; }
    unsigned char *request_body = NULL; size_t request_body_length = 0; if ((request_chunked || request_length >= 0) && !osiris_tls_http_read_body(&connection, timeout, request_headers, request_count, request_chunked, request_length, &request_body, &request_body_length, "http_serve")) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return -1; }
    OValue request_body_value = osiris_tls_http_owned_string(request_body ? request_body : (const unsigned char *)"", request_body_length, "http_serve"); free(request_body); if (request_body_value.kind == O_NULL) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return -1; }
    OValue header_pairs[OSIRIS_TLS_HTTP_MAX_HEADERS]; for (size_t i = 0; i < request_count; i++) { OValue pair[2] = {ostring(request_headers[i].name), ostring(request_headers[i].value)}; header_pairs[i] = olist(pair,2); } OValue request = osiris_http_map_constructor ? osiris_http_map_constructor(osiris_tls_http_userdata, (OirisHTTPMapEntry[]){{"method",ostring(request_method)},{"url",ostring(request_target)},{"headers",olist(header_pairs, request_count)},{"body",request_body_value}},4) : onull();
    OValue response = onull(); if (body_or_handler.kind == O_CLOSURE) { char handler_error[256] = {0}; if (!osiris_tls_http_invoke_handler(body_or_handler, request, &response, handler_error, sizeof(handler_error))) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return (int)(osiris_tls_http_error("http_serve", "", handler_error[0] ? handler_error : "HTTP handler failed"), -1); } }
    const char *response_body = static_body; size_t response_length = static_length; int response_status = status;
    if (body_or_handler.kind == O_CLOSURE) {
      if (!osiris_http_map_lookup) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return (int)(osiris_tls_http_error("http_serve", "", "native HTTP response map lookup is not installed"), -1); }
      OValue field = onull(); if (osiris_http_map_lookup(osiris_tls_http_userdata, response, "status", &field)) { if (field.kind == O_UINT && field.as.u >= 100 && field.as.u <= 599) response_status = (int)field.as.u; else if (field.kind == O_INT && field.as.i >= 100 && field.as.i <= 599) response_status = (int)field.as.i; else { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return (int)(osiris_tls_http_error("http_serve", "", "HTTP handler status is invalid"), -1); } }
      if (osiris_http_map_lookup(osiris_tls_http_userdata, response, "body", &field) && field.kind != O_NULL) { if (field.kind != O_STRING || field.as.s.len > OSIRIS_TLS_HTTP_MAX_BODY) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return (int)(osiris_tls_http_error("http_serve", "", "HTTP handler body is invalid or too large"), -1); } response_body = field.as.s.data; response_length = field.as.s.len; }
    }
    OirisHTTPHeader response_headers[OSIRIS_TLS_HTTP_MAX_HEADERS]; memset(response_headers, 0, sizeof(response_headers)); size_t response_count = 0; if (body_or_handler.kind == O_CLOSURE) { OValue header_field = onull(); if (osiris_http_map_lookup(osiris_tls_http_userdata, response, "headers", &header_field) && !osiris_tls_http_headers(header_field, response_headers, &response_count, "http_serve")) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); return -1; } }
    char response_head[OSIRIS_TLS_HTTP_MAX_LINE]; int head_length = snprintf(response_head, sizeof(response_head), "HTTP/1.1 %d\r\nContent-Length: %zu\r\nConnection: close\r\n", response_status, response_length); if (head_length < 0 || (size_t)head_length >= sizeof(response_head)) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_free_headers(response_headers, response_count); return -1; } size_t response_head_length = (size_t)head_length; for (size_t i = 0; i < response_count; i++) { int part = snprintf(response_head + response_head_length, sizeof(response_head) - response_head_length, "%s: %s\r\n", response_headers[i].name, response_headers[i].value); if (part < 0 || (size_t)part >= sizeof(response_head) - response_head_length) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_free_headers(response_headers, response_count); return -1; } response_head_length += (size_t)part; } if (response_head_length + 2 >= sizeof(response_head)) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_free_headers(response_headers, response_count); return -1; } memcpy(response_head + response_head_length, "\r\n", 2); response_head_length += 2; if (!osiris_tls_http_write_all(&connection, (unsigned char *)response_head, response_head_length, timeout, "http_serve") || !osiris_tls_http_write_all(&connection, (const unsigned char *)response_body, response_length, timeout, "http_serve")) { osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_free_headers(response_headers, response_count); return -1; }
    osiris_tls_http_close(osiris_tls_http_resource_value(&connection)); osiris_tls_http_free_headers(request_headers, request_count); osiris_tls_http_free_headers(response_headers, response_count);
  }
}

static int osiris_native_tls_http_install(const OirisNativeTLSProvider *tls,
                                          const OirisNativeHTTPTransportProvider *transport,
                                          OirisHTTPMapConstructor constructor,
                                          OirisHTTPMapReader reader,
                                          OirisHTTPMapLookup lookup,
                                          OirisHTTPHandler handler, void *userdata) {
  if (tls && tls->abi_version != 1) return 0; if (transport && transport->abi_version != 1) return 0;
#ifdef OSIRIS_OPENSSL_PROVIDER
  if (!tls) tls = osiris_native_openssl_provider();
#endif
  osiris_tls_provider = tls; osiris_http_transport_provider = transport; osiris_http_map_constructor = constructor; osiris_http_map_reader = reader; osiris_http_map_lookup = lookup; osiris_http_handler = handler; osiris_tls_http_userdata = userdata; return 1;
}

static void osiris_native_tls_http_set_cancel(OtlsCancel callback, void *userdata) {
  osiris_tls_http_cancel = callback; osiris_tls_http_cancel_userdata = userdata;
}
`

// nativeTLSHTTPRuntimeSymbols is the function-level contract for the shared
// emitter. The adapter must preserve the hosted std/net argument order,
// passing null for omitted optional arguments.
var nativeTLSHTTPRuntimeSymbols = map[string]string{
	"tls_dial":            "osiris_native_tls_dial(address, server_name_or_null, roots_or_null, timeout_or_null)",
	"tls_dial_config":     "osiris_native_tls_dial(address, server_name, roots_pem, timeout_or_null)",
	"tls_listen":          "osiris_native_tls_listen(address, certificate_path, key_path)",
	"http_get":            "osiris_native_http_get(url, timeout_or_null)",
	"http_request":        "osiris_native_http_request(method, url, headers_or_null, body_or_null, timeout_or_null)",
	"http_request_stream": "osiris_native_http_request_stream_value(method, url, headers_or_null, body_or_null, timeout_or_null)",
	"http_serve":          "osiris_native_http_serve(listener, body_or_handler, status_or_null)",
	"http_body_read":      "osiris_native_http_body_read(body_stream, size, timeout_or_null)",
	"close":               "osiris_tls_http_close(resource)",
}

func nativeTLSHTTPIntegrationSummary() string {
	return "append cRuntimeTLSHTTPComponent; install the default plain-HTTP map/transport bridge; select an explicit TLS provider; preserve nativeTLSHTTPRuntimeSymbols; add closure-handler and redirect parity before promoting the remaining HTTP surface"
}
