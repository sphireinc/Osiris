package osiris

// cRuntimeNativeHTTPBridgeComponent provides the portable plain-HTTP and map
// bridge used by the integrated TLS/HTTP component. TLS itself remains
// provider-backed: no implicit insecure TLS implementation is installed.
const cRuntimeNativeHTTPBridgeComponent = `
#if OSIRIS_NATIVE_POSIX_NET
#if defined(OSIRIS_WINDOWS_COMPONENT)
#define OSIRIS_NATIVE_SOCKET SOCKET
#define OSIRIS_NATIVE_SOCKET_INVALID INVALID_SOCKET
#else
#define OSIRIS_NATIVE_SOCKET int
#define OSIRIS_NATIVE_SOCKET_INVALID (-1)
#endif
static OValue osiris_native_http_map_constructor(void *userdata, const OirisHTTPMapEntry *entries, size_t count) {
  (void)userdata; const char **keys = count ? (const char **)malloc(count * sizeof(char *)) : NULL; OValue *values = count ? (OValue *)malloc(count * sizeof(OValue)) : NULL;
  if (count && (!keys || !values)) { free(keys); free(values); return ofail("out of memory"); }
  for (size_t i=0;i<count;i++) { keys[i] = entries[i].key; values[i] = entries[i].value; }
  OValue result = omap(keys, values, count); free(keys); free(values); return result;
}

static int osiris_native_http_map_reader(void *userdata, OValue value, int (*entry)(void *, const char *, size_t, OValue), void *entry_userdata) {
  (void)userdata; if (value.kind != O_MAP || !value.as.map || !entry) return 0;
  for (size_t i=0;i<value.as.map->len;i++) if (!entry(entry_userdata, value.as.map->entries[i].key.data, value.as.map->entries[i].key.len, value.as.map->entries[i].value)) return 0;
  return 1;
}

static int osiris_native_http_map_lookup(void *userdata, OValue value, const char *key, OValue *output) {
  (void)userdata; OValue *found = omapget(value, key); if (!found || !output) return 0; *output = *found; return 1;
}

static int osiris_native_http_stream_cancel(void *userdata) { (void)userdata; return osiris_tls_http_cancelled(); }

static int64_t osiris_native_http_stream_read(void *state, unsigned char *buffer, size_t capacity, int64_t timeout, OtlsCancel cancelled, void *userdata, char *error, size_t error_capacity) {
  ONetHandle *handle = (ONetHandle *)state; if (!handle || handle->magic != OSIRIS_NET_MAGIC || handle->closed || handle->fd == OSIRIS_NATIVE_SOCKET_INVALID) { osiris_tls_http_copy_error(error,error_capacity,"HTTP transport is closed"); return -1; }
  if (cancelled && cancelled(userdata)) { osiris_tls_http_copy_error(error,error_capacity,"operation cancelled"); return -2; }
  if (!onet_wait(handle->fd, POLLIN, timeout, "http_read", "http")) { osiris_tls_http_copy_error(error,error_capacity,"HTTP read timed out or failed"); return -1; }
  ssize_t result = recv(handle->fd, buffer, capacity, 0); if (result < 0) { osiris_tls_http_copy_error(error,error_capacity,strerror(errno)); return -1; } return (int64_t)result;
}

static int64_t osiris_native_http_stream_write(void *state, const unsigned char *buffer, size_t length, int64_t timeout, OtlsCancel cancelled, void *userdata, char *error, size_t error_capacity) {
  ONetHandle *handle = (ONetHandle *)state; if (!handle || handle->magic != OSIRIS_NET_MAGIC || handle->closed || handle->fd == OSIRIS_NATIVE_SOCKET_INVALID) { osiris_tls_http_copy_error(error,error_capacity,"HTTP transport is closed"); return -1; }
  size_t sent = 0; while (sent < length) { if (cancelled && cancelled(userdata)) { osiris_tls_http_copy_error(error,error_capacity,"operation cancelled"); return -2; } if (!onet_wait(handle->fd, POLLOUT, timeout, "http_write", "http")) { osiris_tls_http_copy_error(error,error_capacity,"HTTP write timed out or failed"); return -1; } ssize_t result = send(handle->fd, buffer + sent, length - sent, 0); if (result < 0) { if (errno == EINTR) continue; osiris_tls_http_copy_error(error,error_capacity,strerror(errno)); return -1; } if (result == 0) break; sent += (size_t)result; } return (int64_t)sent;
}

static int osiris_native_http_stream_close(void *state, char *error, size_t error_capacity) { (void)error; (void)error_capacity; ONetHandle *handle=(ONetHandle *)state; if (!handle || handle->magic != OSIRIS_NET_MAGIC || handle->closed) return 0; int result=close(handle->fd); handle->fd=OSIRIS_NATIVE_SOCKET_INVALID; handle->closed=1; return result==0 ? 0 : -1; }
static void osiris_native_http_stream_destroy(void *state) { (void)state; }

static void osiris_native_http_stream_fill(OirisTLSHTTPStream *stream, ONetHandle *handle) { memset(stream,0,sizeof(*stream)); stream->state=handle; stream->read=osiris_native_http_stream_read; stream->write=osiris_native_http_stream_write; stream->close=osiris_native_http_stream_close; stream->destroy=osiris_native_http_stream_destroy; }

static int osiris_native_http_transport_dial(void *userdata, const char *address, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) { (void)userdata; OSIRIS_NATIVE_SOCKET fd=onet_connect_fd(address,timeout,SOCK_STREAM,"http_dial"); if (fd==OSIRIS_NATIVE_SOCKET_INVALID) { osiris_tls_http_copy_error(error,error_capacity,"unable to connect HTTP transport"); return -1; } ONetHandle *handle=onet_new(fd,OSIRIS_NET_TCP,"http_dial",address); if (!handle) { close(fd); osiris_tls_http_copy_error(error,error_capacity,"out of memory"); return -1; } osiris_native_http_stream_fill(stream,handle); return 0; }
static int osiris_native_http_transport_listen(void *userdata, const char *address, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) { (void)userdata; OValue value=onet_tcp_listen(ostring(address)); if (value.kind != O_RESOURCE || !value.as.resource) { osiris_tls_http_copy_error(error,error_capacity,"unable to listen HTTP transport"); return -1; } osiris_native_http_stream_fill(stream,(ONetHandle *)value.as.resource); return 0; }
static int osiris_native_http_transport_accept(void *userdata, OirisTLSHTTPStream *listener, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) { (void)userdata; ONetHandle *handle=listener?(ONetHandle *)listener->state:NULL; if (!handle || !onet_handle((OValue){O_RESOURCE,0,{.resource=handle}},OSIRIS_NET_TCP_LISTENER,"http_accept")) { osiris_tls_http_copy_error(error,error_capacity,"HTTP listener is closed"); return -1; } if (!onet_wait(handle->fd,POLLIN,timeout,"http_accept","http")) { osiris_tls_http_copy_error(error,error_capacity,"HTTP accept timed out"); return -1; } OSIRIS_NATIVE_SOCKET fd=accept(handle->fd,NULL,NULL); if (fd==OSIRIS_NATIVE_SOCKET_INVALID) { osiris_tls_http_copy_error(error,error_capacity,strerror(errno)); return -1; } ONetHandle *accepted=onet_new(fd,OSIRIS_NET_TCP,"http_accept","accepted HTTP peer"); if (!accepted) { close(fd); osiris_tls_http_copy_error(error,error_capacity,"out of memory"); return -1; } osiris_native_http_stream_fill(stream,accepted); return 0; }

static OirisNativeHTTPTransportProvider osiris_native_http_transport = {1, OSIRIS_HTTP_CAP_CLIENT|OSIRIS_HTTP_CAP_STREAM|OSIRIS_HTTP_CAP_SERVER, NULL, osiris_native_http_transport_dial, osiris_native_http_transport_listen, osiris_native_http_transport_accept};
static int osiris_native_http_defaults_ready = 0;
static void osiris_native_http_ensure_defaults(void) { if (osiris_native_http_defaults_ready) return; osiris_native_tls_http_install(NULL,&osiris_native_http_transport,osiris_native_http_map_constructor,osiris_native_http_map_reader,osiris_native_http_map_lookup,NULL,NULL); osiris_native_http_defaults_ready=1; }

static OValue osiris_native_http_request_default(OValue method, OValue url, OValue headers, OValue body, OValue timeout) { osiris_native_http_ensure_defaults(); return osiris_native_http_request(method,url,headers,body,timeout); }
static OValue osiris_native_http_get_default(OValue url, OValue timeout) { osiris_native_http_ensure_defaults(); return osiris_native_http_get(url,timeout); }
static OValue osiris_native_http_request_stream_default(OValue method, OValue url, OValue headers, OValue body, OValue timeout) { osiris_native_http_ensure_defaults(); return osiris_native_http_request_stream_value(method,url,headers,body,timeout); }
static OValue osiris_native_http_serve_default(OValue listener, OValue body, OValue status) {
  osiris_native_http_ensure_defaults();
  if (listener.kind == O_RESOURCE && listener.as.resource && *((uint64_t *)listener.as.resource) == OSIRIS_NET_MAGIC) {
    ONetHandle *socket = (ONetHandle *)listener.as.resource;
    OirisTLSHTTPResource adapted = {0};
    adapted.magic = OSIRIS_TLS_HTTP_MAGIC;
    adapted.kind = OSIRIS_TLS_HTTP_RESOURCE_LISTENER;
    osiris_native_http_stream_fill(&adapted.stream, socket);
    adapted.secure_listener = 0;
    (void)osiris_native_http_serve(osiris_tls_http_resource_value(&adapted), body, status);
    return onull();
  }
  (void)osiris_native_http_serve(listener, body, status);
  return onull();
}

static OValue osiris_native_resource_close(OValue value) {
  if (value.kind != O_RESOURCE || !value.as.resource) return ofail("close expects an owned native resource");
  uint64_t magic = *((uint64_t *)value.as.resource);
  if (magic == OSIRIS_TLS_HTTP_MAGIC) return osiris_tls_http_close(value);
  return osiris_host_close_any(value);
}

static OValue osiris_native_resource_readchunk(OValue value, OValue size, OValue timeout) {
  if (value.kind != O_RESOURCE || !value.as.resource) return ofail("read expects an owned native resource");
  uint64_t magic = *((uint64_t *)value.as.resource);
  if (magic == OSIRIS_TLS_HTTP_MAGIC) {
    OirisTLSHTTPResource *resource = (OirisTLSHTTPResource *)value.as.resource;
    if (resource->kind == OSIRIS_TLS_HTTP_RESOURCE_BODY) return osiris_native_http_body_read(value, size, timeout);
    return osiris_native_tls_stream_readchunk(value, size, timeout);
  }
  if (magic == OSIRIS_NET_MAGIC) return onet_read(value, size, timeout);
  if (*((uint32_t *)value.as.resource) == OSIRIS_HOST_RESOURCE_MAGIC) {
    OHostResource *resource = (OHostResource *)value.as.resource;
    if (resource->kind == OSIRIS_HOST_WATCH) return osiris_host_watch_read(value, size, timeout);
    if (resource->kind == OSIRIS_HOST_SIGNAL) return osiris_host_signal_read(value, size, timeout);
  }
  return osiris_host_file_io(value, size, onull(), timeout, 0, 0);
}

static OValue osiris_native_resource_writechunk(OValue value, OValue data, OValue timeout) {
  if (value.kind != O_RESOURCE || !value.as.resource) return ofail("write expects an owned native resource");
  uint64_t magic = *((uint64_t *)value.as.resource);
  if (magic == OSIRIS_TLS_HTTP_MAGIC) return osiris_native_tls_stream_writechunk(value, data, timeout);
  if (magic == OSIRIS_NET_MAGIC) return onet_write(value, data, timeout);
  return osiris_host_file_io(value, data, onull(), timeout, 1, 0);
}
#else
static OValue osiris_native_http_get_default(OValue url, OValue timeout) { (void)url; (void)timeout; return onet_error("http_get", "", "native HTTP is unavailable on this platform"); }
static OValue osiris_native_http_request_default(OValue method, OValue url, OValue headers, OValue body, OValue timeout) { (void)method; (void)url; (void)headers; (void)body; (void)timeout; return onet_error("http_request", "", "native HTTP is unavailable on this platform"); }
static OValue osiris_native_http_request_stream_default(OValue method, OValue url, OValue headers, OValue body, OValue timeout) { (void)method; (void)url; (void)headers; (void)body; (void)timeout; return onet_error("http_request_stream", "", "native HTTP is unavailable on this platform"); }
static OValue osiris_native_http_serve_default(OValue listener, OValue body, OValue status) { (void)listener; (void)body; (void)status; return onet_error("http_serve", "", "native HTTP is unavailable on this platform"); }
static OValue osiris_native_resource_close(OValue value) { (void)value; return onet_error("close", "", "native host resources are unavailable on this platform"); }
static OValue osiris_native_resource_readchunk(OValue value, OValue size, OValue timeout) { (void)value; (void)size; (void)timeout; return onet_error("readchunk", "", "native host resources are unavailable on this platform"); }
static OValue osiris_native_resource_writechunk(OValue value, OValue data, OValue timeout) { (void)value; (void)data; (void)timeout; return onet_error("writechunk", "", "native host resources are unavailable on this platform"); }
#endif
`
