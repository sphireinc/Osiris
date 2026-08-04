package osiris

// cRuntimeOpenSSLProviderComponent is the opt-in native TLS provider used
// when the generated executable is compiled with OpenSSL discovered by
// pkg-config. It is guarded in C so ordinary native programs and plain HTTP do
// not acquire an OpenSSL dependency. Verification is always enabled.
const cRuntimeOpenSSLProviderComponent = `
#ifdef OSIRIS_OPENSSL_PROVIDER
#include <openssl/err.h>
#include <openssl/ssl.h>
#include <openssl/pem.h>
#include <openssl/x509.h>
#include <sys/time.h>

typedef struct {
  SSL_CTX *ctx;
  SSL *ssl;
  int fd;
  int listener;
} OOpenSSLState;

static void osiris_openssl_error(char *error, size_t capacity, const char *fallback) {
  unsigned long code = ERR_get_error();
  if (code) ERR_error_string_n(code, error, capacity);
  else osiris_tls_http_copy_error(error, capacity, fallback);
}

static int osiris_openssl_timeout(int fd, int64_t timeout_ms) {
  if (timeout_ms <= 0) return 1;
  struct timeval value = {(time_t)(timeout_ms / 1000), (suseconds_t)((timeout_ms % 1000) * 1000)};
  return setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &value, sizeof(value)) == 0 && setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &value, sizeof(value)) == 0;
}

static int64_t osiris_openssl_read(void *state, unsigned char *buffer, size_t capacity, int64_t timeout, OtlsCancel cancelled, void *userdata, char *error, size_t error_capacity) {
  OOpenSSLState *connection = (OOpenSSLState *)state;
  if (!connection || !connection->ssl || (cancelled && cancelled(userdata))) { osiris_tls_http_copy_error(error, error_capacity, "TLS stream is closed or cancelled"); return -2; }
  if (!onet_wait(connection->fd, POLLIN, timeout, "tls_read", "tls")) { osiris_tls_http_copy_error(error, error_capacity, "TLS read timed out or failed"); return -1; }
  int count = SSL_read(connection->ssl, buffer, (int)(capacity > INT_MAX ? INT_MAX : capacity));
  if (count > 0) return count;
  int code = SSL_get_error(connection->ssl, count);
  if (code == SSL_ERROR_ZERO_RETURN) return 0;
  osiris_openssl_error(error, error_capacity, "TLS read failed"); return -1;
}

static int64_t osiris_openssl_write(void *state, const unsigned char *buffer, size_t length, int64_t timeout, OtlsCancel cancelled, void *userdata, char *error, size_t error_capacity) {
  OOpenSSLState *connection = (OOpenSSLState *)state;
  if (!connection || !connection->ssl || (cancelled && cancelled(userdata))) { osiris_tls_http_copy_error(error, error_capacity, "TLS stream is closed or cancelled"); return -2; }
  if (!onet_wait(connection->fd, POLLOUT, timeout, "tls_write", "tls")) { osiris_tls_http_copy_error(error, error_capacity, "TLS write timed out or failed"); return -1; }
  int count = SSL_write(connection->ssl, buffer, (int)(length > INT_MAX ? INT_MAX : length));
  if (count > 0) return count;
  osiris_openssl_error(error, error_capacity, "TLS write failed"); return -1;
}

static int osiris_openssl_close(void *state, char *error, size_t error_capacity) {
  (void)error; (void)error_capacity;
  OOpenSSLState *connection = (OOpenSSLState *)state;
  if (!connection) return 0;
  if (connection->ssl) { (void)SSL_shutdown(connection->ssl); SSL_free(connection->ssl); connection->ssl = NULL; }
  if (connection->ctx) { SSL_CTX_free(connection->ctx); connection->ctx = NULL; }
  if (connection->fd >= 0) { close(connection->fd); connection->fd = -1; }
  connection->listener = 0; return 0;
}

static void osiris_openssl_destroy(void *state) { OOpenSSLState *connection = (OOpenSSLState *)state; if (connection) { osiris_openssl_close(connection, NULL, 0); free(connection); } }
static void osiris_openssl_stream(OirisTLSHTTPStream *stream, OOpenSSLState *state) { memset(stream, 0, sizeof(*stream)); stream->state = state; stream->read = osiris_openssl_read; stream->write = osiris_openssl_write; stream->close = osiris_openssl_close; stream->destroy = osiris_openssl_destroy; }

static int osiris_openssl_roots(SSL_CTX *ctx, const char *roots, char *error, size_t error_capacity) {
  SSL_CTX_set_verify(ctx, SSL_VERIFY_PEER, NULL);
  if (!roots || !*roots) { if (SSL_CTX_set_default_verify_paths(ctx) != 1) { osiris_openssl_error(error, error_capacity, "unable to load platform trust roots"); return 0; } return 1; }
  BIO *bio = BIO_new_mem_buf(roots, -1); if (!bio) { osiris_tls_http_copy_error(error, error_capacity, "out of memory"); return 0; }
  X509_STORE *store = SSL_CTX_get_cert_store(ctx); int count = 0; X509 *certificate = NULL;
  while ((certificate = PEM_read_bio_X509(bio, NULL, NULL, NULL)) != NULL) { if (X509_STORE_add_cert(store, certificate) == 1) count++; X509_free(certificate); }
  BIO_free(bio); if (!count) { osiris_tls_http_copy_error(error, error_capacity, "no valid certificate roots supplied"); return 0; } return 1;
}

static int osiris_openssl_dial(void *userdata, const char *address, const char *server_name, const char *roots, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) {
  (void)userdata; int fd = onet_connect_fd(address, timeout, SOCK_STREAM, "tls_dial"); if (fd < 0) { osiris_tls_http_copy_error(error, error_capacity, "unable to connect TLS transport"); return -1; }
  OOpenSSLState *connection = (OOpenSSLState *)calloc(1, sizeof(*connection)); if (!connection) { close(fd); osiris_tls_http_copy_error(error, error_capacity, "out of memory"); return -1; }
  connection->fd = fd; connection->ctx = SSL_CTX_new(TLS_client_method()); if (!connection->ctx || SSL_CTX_set_min_proto_version(connection->ctx, TLS1_2_VERSION) != 1 || !osiris_openssl_timeout(fd, timeout) || !osiris_openssl_roots(connection->ctx, roots, error, error_capacity)) { osiris_openssl_destroy(connection); if (!error[0]) osiris_openssl_error(error, error_capacity, "unable to initialize TLS client"); return -1; }
  connection->ssl = SSL_new(connection->ctx); if (!connection->ssl) { osiris_openssl_destroy(connection); osiris_openssl_error(error, error_capacity, "unable to allocate TLS session"); return -1; }
  if (server_name && *server_name) { if (SSL_set_tlsext_host_name(connection->ssl, server_name) != 1 || SSL_set1_host(connection->ssl, server_name) != 1) { osiris_openssl_error(error, error_capacity, "unable to configure TLS server name"); osiris_openssl_destroy(connection); return -1; } }
  static const unsigned char protocols[] = {8,'h','t','t','p','/','1','.','1'}; SSL_set_alpn_protos(connection->ssl, protocols, sizeof(protocols)); SSL_set_fd(connection->ssl, fd);
  if (SSL_connect(connection->ssl) != 1) { osiris_openssl_error(error, error_capacity, "TLS handshake failed"); osiris_openssl_destroy(connection); return -1; }
  osiris_openssl_stream(stream, connection); return 0;
}

static int osiris_openssl_listen(void *userdata, const char *address, const char *certificate, const char *key, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) {
  (void)userdata; OValue listener_value = onet_tcp_listen(ostring(address)); ONetHandle *network = onet_handle(listener_value, OSIRIS_NET_TCP_LISTENER, "tls_listen"); if (!network) { osiris_tls_http_copy_error(error, error_capacity, "unable to bind TLS listener"); return -1; }
  OOpenSSLState *listener = (OOpenSSLState *)calloc(1, sizeof(*listener)); if (!listener) { (void)onet_close(listener_value); osiris_tls_http_copy_error(error, error_capacity, "out of memory"); return -1; }
  listener->fd = network->fd; listener->listener = 1; network->fd = -1; network->closed = 1; listener->ctx = SSL_CTX_new(TLS_server_method());
  if (!listener->ctx || SSL_CTX_use_certificate_file(listener->ctx, certificate, SSL_FILETYPE_PEM) != 1 || SSL_CTX_use_PrivateKey_file(listener->ctx, key, SSL_FILETYPE_PEM) != 1 || SSL_CTX_check_private_key(listener->ctx) != 1) { osiris_openssl_error(error, error_capacity, "unable to load TLS certificate or key"); osiris_openssl_destroy(listener); return -1; }
  SSL_CTX_set_min_proto_version(listener->ctx, TLS1_2_VERSION); osiris_openssl_stream(stream, listener); return 0;
}

static int osiris_openssl_accept(void *userdata, OirisTLSHTTPStream *listener_stream, int64_t timeout, OirisTLSHTTPStream *stream, char *error, size_t error_capacity) {
  (void)userdata; OOpenSSLState *listener = listener_stream ? (OOpenSSLState *)listener_stream->state : NULL; if (!listener || !listener->listener || !listener->ctx || !onet_wait(listener->fd, POLLIN, timeout, "tls_accept", "tls")) { osiris_tls_http_copy_error(error, error_capacity, "TLS accept timed out or failed"); return -1; }
  int fd = accept(listener->fd, NULL, NULL); if (fd < 0) { osiris_openssl_error(error, error_capacity, "TLS accept failed"); return -1; }
  OOpenSSLState *connection = (OOpenSSLState *)calloc(1, sizeof(*connection)); if (!connection) { close(fd); osiris_tls_http_copy_error(error, error_capacity, "out of memory"); return -1; }
  connection->fd = fd; connection->ctx = listener->ctx; SSL_CTX_up_ref(connection->ctx); connection->ssl = SSL_new(connection->ctx); if (!connection->ssl) { osiris_openssl_destroy(connection); osiris_openssl_error(error, error_capacity, "unable to allocate TLS session"); return -1; } SSL_set_fd(connection->ssl, fd);
  if (SSL_accept(connection->ssl) != 1) { osiris_openssl_error(error, error_capacity, "TLS handshake failed"); osiris_openssl_destroy(connection); return -1; }
  osiris_openssl_stream(stream, connection); return 0;
}

static OirisNativeTLSProvider osiris_openssl_provider = {1, OSIRIS_TLS_CAP_DIAL|OSIRIS_TLS_CAP_LISTEN, NULL, osiris_openssl_dial, osiris_openssl_listen, osiris_openssl_accept};
static const OirisNativeTLSProvider *osiris_native_openssl_provider(void) { return &osiris_openssl_provider; }
#endif
`
