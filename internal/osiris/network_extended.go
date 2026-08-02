package osiris

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxUDPPacketBytes = 1 << 20
	maxHTTPBodyBytes  = 8 << 20
	maxHTTPHeaders    = 256
)

// udpSocket is an owned packet endpoint. A socket can be connected (in which
// case udp_send takes two arguments) or unconnected (udp_send takes an
// explicit peer address). It deliberately remains distinct from streamValue:
// packet boundaries, peer identity, and truncation are observable semantics.
type udpSocket struct {
	mu   sync.Mutex
	conn *net.UDPConn
}

func (s *udpSocket) isOpen() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	open := s.conn != nil
	s.mu.Unlock()
	return open
}

func (s *udpSocket) address() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil || conn.LocalAddr() == nil {
		return ""
	}
	return conn.LocalAddr().String()
}

func (s *udpSocket) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func udpSocketFromValue(value Value) (*udpSocket, error) {
	if value.kind != kindUDPSocket || value.udp == nil {
		return nil, fmt.Errorf("expected udp socket, got %s", value.Kind())
	}
	value.udp.mu.Lock()
	open := value.udp.conn != nil
	value.udp.mu.Unlock()
	if !open {
		return nil, fmt.Errorf("udp socket is closed")
	}
	return value.udp, nil
}

func udpBindContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("udp_bind expects an address")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	var udpAddress *net.UDPAddr
	if address != "" {
		udpAddress, err = net.ResolveUDPAddr("udp", address)
		if err != nil {
			return nullValue(), err
		}
	}
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return nullValue(), err
	}
	return udpSocketValueOf(&udpSocket{conn: conn}), nil
}

func udpConnectContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("udp_connect expects a peer address")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	peer, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nullValue(), err
	}
	conn, err := net.DialUDP("udp", nil, peer)
	if err != nil {
		return nullValue(), err
	}
	return udpSocketValueOf(&udpSocket{conn: conn}), nil
}

func udpSendContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nullValue(), fmt.Errorf("udp_send expects socket, data, and optional peer address")
	}
	socket, err := udpSocketFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	data, err := bytesFromValue(args[1])
	if err != nil {
		return nullValue(), fmt.Errorf("udp_send: %w", err)
	}
	if len(data) > maxUDPPacketBytes {
		return nullValue(), fmt.Errorf("udp_send packet exceeds %d bytes", maxUDPPacketBytes)
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	socket.mu.Lock()
	conn := socket.conn
	socket.mu.Unlock()
	if conn == nil {
		return nullValue(), fmt.Errorf("udp socket is closed")
	}
	var written int
	if len(args) == 3 {
		address, err := args[2].AsString()
		if err != nil {
			return nullValue(), err
		}
		peer, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			return nullValue(), err
		}
		written, err = conn.WriteToUDP(data, peer)
	} else {
		written, err = conn.Write(data)
	}
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(written), 64), nil
}

func udpReceiveContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return nullValue(), fmt.Errorf("udp_receive expects socket and optional max bytes and timeout")
	}
	socket, err := udpSocketFromValue(args[0])
	if err != nil {
		return nullValue(), err
	}
	limit := maxUDPPacketBytes
	if len(args) >= 2 {
		value, err := filesystemNonnegativeArgument(args[1], "udp_receive", "max_bytes", maxUDPPacketBytes)
		if err != nil {
			return nullValue(), err
		}
		limit = int(value)
	}
	timeout := 0
	if len(args) == 3 {
		value, err := filesystemNonnegativeArgument(args[2], "udp_receive", "timeout_ms", uint64(^uint(0)>>1))
		if err != nil {
			return nullValue(), err
		}
		timeout = int(value)
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	socket.mu.Lock()
	conn := socket.conn
	socket.mu.Unlock()
	if conn == nil {
		return nullValue(), fmt.Errorf("udp socket is closed")
	}
	deadline := tcpDeadline(ctx, timeout)
	if !deadline.IsZero() {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nullValue(), err
		}
		defer conn.SetReadDeadline(time.Time{})
	} else if ctx.Done() != nil {
		// A context without a deadline still has to interrupt a blocked packet
		// read. The watcher only exists for this unbounded operation and is
		// stopped before the socket is reused by the caller.
		stopped := make(chan struct{})
		exited := make(chan struct{})
		go func() {
			defer close(exited)
			select {
			case <-ctx.Done():
				_ = conn.SetReadDeadline(time.Now())
			case <-stopped:
			}
		}()
		defer func() {
			close(stopped)
			<-exited
		}()
	}
	buffer := make([]byte, limit)
	n, peer, err := conn.ReadFromUDP(buffer)
	if err != nil {
		if ctx.Err() != nil {
			return nullValue(), ctx.Err()
		}
		return nullValue(), err
	}
	// ReadFromUDP exposes the bytes retained by the kernel. Equality with the
	// caller's bound is the portable truncation signal; callers must use a
	// larger bound when they need to distinguish an exactly-full packet.
	truncated := n == limit
	return tupleValue([]Value{
		bytesToValue(buffer[:n]),
		stringValue(peer.String()),
		boolValue(truncated),
	}), nil
}

func bytesToValue(data []byte) Value {
	items := make([]Value, len(data))
	for i, item := range data {
		items[i] = uintValue(uint64(item), 8)
	}
	return listValue(items)
}

func closeUDPSocketValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("close expects 1 argument")
	}
	socket, err := udpSocketFromValue(args[0])
	if err != nil {
		if args[0].kind == kindUDPSocket && args[0].udp != nil && !args[0].udp.isOpen() {
			return boolValue(false), nil
		}
		return nullValue(), err
	}
	if err := socket.close(); err != nil {
		return nullValue(), err
	}
	return boolValue(true), nil
}

func dnsLookupContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("dns_lookup expects host and optional resolver policy")
	}
	host, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	policy := "system"
	if len(args) == 2 {
		policy, err = args[1].AsString()
		if err != nil {
			return nullValue(), err
		}
	}
	if policy != "system" && policy != "go" {
		return nullValue(), fmt.Errorf("dns_lookup resolver policy must be %q or %q", "system", "go")
	}
	resolver := net.Resolver{PreferGo: policy == "go"}
	values, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return nullValue(), networkFailure("dns_lookup", host, err)
	}
	sort.Strings(values)
	unique := values[:0]
	for _, value := range values {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	result := make([]Value, len(unique))
	for i, value := range unique {
		result[i] = stringValue(value)
	}
	return listValue(result), nil
}

func tlsDialContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 3 {
		return nullValue(), fmt.Errorf("tls_dial expects address and optional server name and timeout")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	serverName := ""
	if len(args) >= 2 {
		serverName, err = args[1].AsString()
		if err != nil {
			return nullValue(), err
		}
	}
	if serverName == "" {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr == nil {
			serverName = host
		} else {
			serverName = address
		}
	}
	timeout := 0
	if len(args) == 3 {
		value, err := filesystemNonnegativeArgument(args[2], "tls_dial", "timeout_ms", uint64(^uint(0)>>1))
		if err != nil {
			return nullValue(), err
		}
		timeout = int(value)
	}
	config := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	dialer := &tls.Dialer{Config: config}
	if timeout > 0 {
		deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
		if existing, ok := ctx.Deadline(); !ok || deadline.Before(existing) {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
	}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nullValue(), networkFailure("tls_dial", address, err)
	}
	stream, err := tcpStreamWithConfig(connection, DefaultSocketConfig())
	if err != nil {
		_ = connection.Close()
		return nullValue(), networkFailure("tls_dial", address, err)
	}
	stream.mode = "tls"
	return streamValueOf(stream), nil
}

// tlsDialConfigContextValue is the explicit-root client form. Roots are PEM
// data supplied by the caller; the runtime never enables insecure verification
// and never consults an ambient mutable trust setting.
func tlsDialConfigContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 3 || len(args) > 4 {
		return nullValue(), fmt.Errorf("tls_dial_config expects address, server name, roots PEM, and optional timeout")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	serverName, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	if strings.TrimSpace(serverName) == "" {
		return nullValue(), networkFailure("tls_dial_config", address, fmt.Errorf("server name must not be empty"))
	}
	rootsPEM, err := args[2].AsString()
	if err != nil {
		return nullValue(), err
	}
	pool := x509.NewCertPool()
	if rootsPEM == "" || !pool.AppendCertsFromPEM([]byte(rootsPEM)) {
		return nullValue(), networkFailure("tls_dial_config", address, fmt.Errorf("no valid certificate roots supplied"))
	}
	timeout := 0
	if len(args) == 4 {
		value, timeoutErr := filesystemNonnegativeArgument(args[3], "tls_dial_config", "timeout_ms", uint64(^uint(0)>>1))
		if timeoutErr != nil {
			return nullValue(), timeoutErr
		}
		timeout = int(value)
	}
	config := &tls.Config{ServerName: serverName, RootCAs: pool, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	dialer := &tls.Dialer{Config: config}
	if timeout > 0 {
		deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
		if existing, ok := ctx.Deadline(); !ok || deadline.Before(existing) {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
	}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nullValue(), networkFailure("tls_dial_config", address, err)
	}
	stream, err := tcpStreamWithConfig(connection, DefaultSocketConfig())
	if err != nil {
		_ = connection.Close()
		return nullValue(), networkFailure("tls_dial_config", address, err)
	}
	stream.mode = "tls"
	return streamValueOf(stream), nil
}

// tlsListenContextValue builds a TLS-wrapped listener with TLS 1.2 as the
// minimum protocol and HTTP/1.1 as the only advertised ALPN protocol. Paths
// must be absolute so a working-directory change cannot redirect a server to
// a different certificate pair.
func tlsListenContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("tls_listen expects address, certificate path, and key path")
	}
	address, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	certPath, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	keyPath, err := args[2].AsString()
	if err != nil {
		return nullValue(), err
	}
	if !filepath.IsAbs(certPath) || !filepath.IsAbs(keyPath) {
		return nullValue(), networkFailure("tls_listen", address, fmt.Errorf("certificate and key paths must be absolute"))
	}
	if err := ctx.Err(); err != nil {
		return nullValue(), err
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nullValue(), networkFailure("tls_listen", address, err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nullValue(), networkFailure("tls_listen", address, err)
	}
	config := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	return tcpListenerValueOf(&tcpListener{listener: tls.NewListener(listener, config)}), nil
}

func headerMap(args Value) (http.Header, error) {
	if args.kind != kindMap || args.mapValue == nil {
		return nil, fmt.Errorf("headers must be a map")
	}
	header := make(http.Header)
	if len(args.mapValue.entries) > maxHTTPHeaders {
		return nil, fmt.Errorf("too many HTTP headers")
	}
	for _, entry := range args.mapValue.entries {
		name, err := entry.key.AsString()
		if err != nil || !validHTTPHeaderName(name) {
			return nil, fmt.Errorf("invalid HTTP header name")
		}
		value, err := entry.value.AsString()
		if err != nil || !validHTTPHeaderValue(value) {
			return nil, fmt.Errorf("invalid HTTP header value")
		}
		header.Set(name, value)
	}
	return header, nil
}

func validHTTPHeaderName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		switch c {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		default:
			return false
		}
	}
	return true
}

func validHTTPHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] == '\r' || value[i] == '\n' || value[i] == 0 || (value[i] < 0x20 && value[i] != '\t') || value[i] == 0x7f {
			return false
		}
	}
	return true
}

func validateHTTPResponseHeaders(response *http.Response) error {
	if response == nil {
		return fmt.Errorf("missing HTTP response")
	}
	if len(response.Header) > maxHTTPHeaders {
		return fmt.Errorf("HTTP response has more than %d headers", maxHTTPHeaders)
	}
	for name, values := range response.Header {
		if !validHTTPHeaderName(name) || len(values) > maxHTTPHeaders {
			return fmt.Errorf("invalid or excessive HTTP response headers")
		}
		for _, value := range values {
			if !validHTTPHeaderValue(value) {
				return fmt.Errorf("invalid HTTP response header value")
			}
		}
	}
	return nil
}

func httpResponseValue(response *http.Response, body []byte) Value {
	entries := make([]Value, 0, len(response.Header))
	keys := make([]string, 0, len(response.Header))
	for key := range response.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := append([]string(nil), response.Header.Values(key)...)
		sort.Strings(values)
		for _, value := range values {
			entries = append(entries, tupleValue([]Value{stringValue(key), stringValue(value)}))
		}
	}
	headerList := listValue(entries)
	return mapValueOf([]mapEntry{
		{key: stringValue("status"), value: uintValue(uint64(response.StatusCode), 16)},
		{key: stringValue("headers"), value: headerList},
		{key: stringValue("body"), value: stringValue(string(body))},
		{key: stringValue("url"), value: stringValue(response.Request.URL.String())},
	})
}

type streamBodyReader struct{ stream *streamValue }

func (reader streamBodyReader) Read(buffer []byte) (int, error) {
	if reader.stream == nil {
		return 0, io.EOF
	}
	reader.stream.readMu.Lock()
	defer reader.stream.readMu.Unlock()
	reader.stream.mu.Lock()
	if err := reader.stream.ensureOpen(); err != nil {
		reader.stream.mu.Unlock()
		return 0, err
	}
	underlying := reader.stream.reader
	reader.stream.mu.Unlock()
	if underlying == nil {
		return 0, fmt.Errorf("stream is not readable")
	}
	return underlying.Read(buffer)
}

type boundedRequestBody struct {
	reader   io.Reader
	read     int64
	tooLarge bool
}

func (body *boundedRequestBody) Read(buffer []byte) (int, error) {
	if body.tooLarge {
		return 0, fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPBodyBytes)
	}
	if body.read == maxHTTPBodyBytes {
		var probe [1]byte
		n, err := body.reader.Read(probe[:])
		if n > 0 {
			body.tooLarge = true
			return 0, fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPBodyBytes)
		}
		return 0, err
	}
	remaining := maxHTTPBodyBytes - body.read
	if int64(len(buffer)) > remaining {
		buffer = buffer[:remaining]
	}
	n, err := body.reader.Read(buffer)
	body.read += int64(n)
	return n, err
}

type boundedResponseBody struct {
	reader   io.Reader
	closer   io.Closer
	read     int64
	tooLarge bool
}

func (body *boundedResponseBody) Read(buffer []byte) (int, error) {
	if body.tooLarge {
		return 0, fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
	}
	if body.read == maxHTTPBodyBytes {
		var probe [1]byte
		n, err := body.reader.Read(probe[:])
		if n > 0 {
			body.tooLarge = true
			return 0, fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
		}
		return 0, err
	}
	remaining := maxHTTPBodyBytes - body.read
	if int64(len(buffer)) > remaining {
		buffer = buffer[:remaining]
	}
	n, err := body.reader.Read(buffer)
	body.read += int64(n)
	return n, err
}

func (body *boundedResponseBody) Close() error { return body.closer.Close() }

func httpRequestStreamContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 5 {
		return nullValue(), fmt.Errorf("http_request_stream expects method, url, optional headers, body, and timeout")
	}
	method, err := args[0].AsString()
	if err != nil || strings.TrimSpace(method) == "" {
		return nullValue(), fmt.Errorf("http_request_stream method must be a non-empty string")
	}
	url, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	var headers http.Header
	if len(args) >= 3 && args[2].kind != kindNull {
		headers, err = headerMap(args[2])
		if err != nil {
			return nullValue(), err
		}
	}
	if headers == nil {
		headers = make(http.Header)
	}
	var requestBody io.Reader
	if len(args) >= 4 && args[3].kind != kindNull {
		if args[3].kind == kindStream {
			requestBody = &boundedRequestBody{reader: streamBodyReader{stream: args[3].stream}}
		} else {
			data, dataErr := bytesFromValue(args[3])
			if dataErr != nil {
				return nullValue(), dataErr
			}
			if len(data) > maxHTTPBodyBytes {
				return nullValue(), fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPBodyBytes)
			}
			requestBody = bytes.NewReader(data)
		}
	}
	if len(args) == 5 {
		timeout, timeoutErr := filesystemNonnegativeArgument(args[4], "http_request_stream", "timeout_ms", uint64(^uint(0)>>1))
		if timeoutErr != nil {
			return nullValue(), timeoutErr
		}
		if timeout > 0 {
			deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
			if existing, ok := ctx.Deadline(); !ok || deadline.Before(existing) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, deadline)
				defer cancel()
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, url, requestBody)
	if err != nil {
		return nullValue(), err
	}
	request.Header = headers
	client := &http.Client{CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("HTTP redirect limit exceeded")
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return nullValue(), networkFailure("http_request_stream", url, err)
	}
	if err := validateHTTPResponseHeaders(response); err != nil {
		_ = response.Body.Close()
		return nullValue(), networkFailure("http_request_stream", url, err)
	}
	body := &boundedResponseBody{reader: response.Body, closer: response.Body}
	stream := &streamValue{id: response.Request.URL.String(), path: response.Request.URL.String(), mode: "http", reader: bufio.NewReader(body), closer: body}
	return httpResponseValueWithBody(response, streamValueOf(stream)), nil
}

func httpResponseValueWithBody(response *http.Response, body Value) Value {
	value := httpResponseValue(response, nil)
	value.mapValue.entries = append(value.mapValue.entries, mapEntry{key: stringValue("body_stream"), value: body})
	return value
}

func httpRequestContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 5 {
		return nullValue(), fmt.Errorf("http_request expects method, url, optional headers, body, and timeout")
	}
	method, err := args[0].AsString()
	if err != nil || strings.TrimSpace(method) == "" {
		return nullValue(), fmt.Errorf("http_request method must be a non-empty string")
	}
	url, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	var headers http.Header
	body := ""
	if len(args) >= 3 && args[2].kind != kindNull {
		headers, err = headerMap(args[2])
		if err != nil {
			return nullValue(), err
		}
	}
	if headers == nil {
		headers = make(http.Header)
	}
	if len(args) >= 4 && args[3].kind != kindNull {
		data, dataErr := bytesFromValue(args[3])
		if dataErr != nil {
			return nullValue(), dataErr
		}
		if len(data) > maxHTTPBodyBytes {
			return nullValue(), fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPBodyBytes)
		}
		body = string(data)
	}
	if len(args) == 5 {
		timeout, timeoutErr := filesystemNonnegativeArgument(args[4], "http_request", "timeout_ms", uint64(^uint(0)>>1))
		if timeoutErr != nil {
			return nullValue(), timeoutErr
		}
		if timeout > 0 {
			deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
			if existing, ok := ctx.Deadline(); !ok || deadline.Before(existing) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, deadline)
				defer cancel()
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return nullValue(), err
	}
	request.Header = headers
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("HTTP redirect limit exceeded")
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nullValue(), networkFailure("http_request", url, err)
	}
	defer response.Body.Close()
	if err := validateHTTPResponseHeaders(response); err != nil {
		return nullValue(), networkFailure("http_request", url, err)
	}
	reader := io.LimitReader(response.Body, maxHTTPBodyBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nullValue(), networkFailure("http_request", url, err)
	}
	if len(data) > maxHTTPBodyBytes {
		return nullValue(), fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
	}
	return httpResponseValue(response, data), nil
}

func httpGetContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("http_get expects url and optional timeout")
	}
	requestArgs := []Value{stringValue("GET"), args[0], mapValueOf(nil), nullValue()}
	if len(args) == 2 {
		requestArgs = append(requestArgs, args[1])
	}
	return httpRequestContextValue(ctx, requestArgs)
}

func httpMapField(value Value, name string) (Value, bool) {
	if value.kind != kindMap || value.mapValue == nil {
		return nullValue(), false
	}
	for _, entry := range value.mapValue.entries {
		if entry.key.kind == kindString && entry.key.str == name {
			return entry.value, true
		}
	}
	return nullValue(), false
}

func httpRequestValue(request *http.Request) (Value, error) {
	if request == nil {
		return nullValue(), fmt.Errorf("missing HTTP request")
	}
	var body []byte
	var err error
	if request.Body != nil {
		body, err = io.ReadAll(io.LimitReader(request.Body, maxHTTPBodyBytes+1))
		if closeErr := request.Body.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return nullValue(), err
	}
	if len(body) > maxHTTPBodyBytes {
		return nullValue(), fmt.Errorf("HTTP request body exceeds %d bytes", maxHTTPBodyBytes)
	}
	keys := make([]string, 0, len(request.Header))
	for key := range request.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]Value, 0, len(keys))
	for _, key := range keys {
		values := append([]string(nil), request.Header.Values(key)...)
		sort.Strings(values)
		for _, value := range values {
			entries = append(entries, tupleValue([]Value{stringValue(key), stringValue(value)}))
		}
	}
	return mapValueOf([]mapEntry{
		{key: stringValue("method"), value: stringValue(request.Method)},
		{key: stringValue("url"), value: stringValue(request.URL.String())},
		{key: stringValue("headers"), value: listValue(entries)},
		{key: stringValue("body"), value: stringValue(string(body))},
		{key: stringValue("remote_addr"), value: stringValue(request.RemoteAddr)},
	}), nil
}

func invokeHTTPHandler(ctx context.Context, handler, request Value) (Value, error) {
	if handler.kind != kindFunction || handler.fn == nil {
		return nullValue(), fmt.Errorf("HTTP handler must be a function")
	}
	if err := contextError(ctx); err != nil {
		return nullValue(), err
	}
	if handler.fn.builtin != nil {
		return handler.fn.builtin([]Value{request})
	}
	if handler.fn.contextBuiltin != nil {
		return handler.fn.contextBuiltin(ctx, []Value{request})
	}
	session := handler.fn.session
	if session == nil {
		return nullValue(), fmt.Errorf("HTTP handler has no execution session")
	}
	// A user closure retains its lexical environment. Serialize requests and
	// temporarily bind the request context so captured state cannot be mutated
	// concurrently by multiple net/http handler goroutines.
	session.networkMu.Lock()
	defer session.networkMu.Unlock()
	previousContext, previousEvaluatorContext := session.ctx, session.ev.ctx
	previousEmitAllowed := session.emitAllowed
	session.ctx, session.ev.ctx = ctx, ctx
	session.emitAllowed = false
	defer func() {
		session.ctx, session.ev.ctx = previousContext, previousEvaluatorContext
		session.emitAllowed = previousEmitAllowed
	}()
	return session.eval(&callExpr{
		callee: &literalExpr{value: handler},
		args:   []Expr{&literalExpr{value: request}},
	}, session.scope)
}

func httpHandlerResponse(value Value) (int, http.Header, []byte, error) {
	if value.kind != kindMap || value.mapValue == nil {
		return 0, nil, nil, fmt.Errorf("HTTP handler must return a response map")
	}
	status := 200
	if field, ok := httpMapField(value, "status"); ok {
		parsed, err := field.Int()
		if err != nil || parsed < 100 || parsed > 599 {
			return 0, nil, nil, fmt.Errorf("HTTP response status must be between 100 and 599")
		}
		status = parsed
	}
	header := make(http.Header)
	if field, ok := httpMapField(value, "headers"); ok {
		var err error
		header, err = headerMap(field)
		if err != nil {
			return 0, nil, nil, err
		}
	}
	body := []byte{}
	if field, ok := httpMapField(value, "body"); ok && field.kind != kindNull {
		var err error
		body, err = bytesFromValue(field)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("HTTP response body: %w", err)
		}
		if len(body) > maxHTTPBodyBytes {
			return 0, nil, nil, fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
		}
	}
	return status, header, body, nil
}

// httpServeContextValue supports both the backwards-compatible static form
// `http_serve(listener, body[, status])` and a bounded dynamic form whose
// handler receives a request map and returns a response map. Handler calls are
// serialized per Osiris session, cancellation closes the server, and emits are
// forbidden from background request goroutines.
func httpServeContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nullValue(), fmt.Errorf("http_serve expects listener, handler/body, and optional status")
	}
	listener, err := args[0].AsTCPListener()
	if err != nil {
		return nullValue(), err
	}
	dynamic := args[1].kind == kindFunction
	body := ""
	status := 200
	if !dynamic {
		body, err = args[1].AsString()
		if err != nil {
			return nullValue(), err
		}
		if len(args) == 3 {
			status, err = args[2].Int()
			if err != nil || status < 100 || status > 599 {
				return nullValue(), fmt.Errorf("http_serve status must be between 100 and 599")
			}
		}
	} else if len(args) == 3 {
		return nullValue(), fmt.Errorf("dynamic http_serve accepts only listener and handler")
	}
	listener.mu.Lock()
	netListener := listener.listener
	listener.mu.Unlock()
	if netListener == nil {
		return nullValue(), fmt.Errorf("tcp listener is closed")
	}
	server := &http.Server{MaxHeaderBytes: 1 << 20}
	server.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !dynamic {
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, body)
			return
		}
		requestValue, requestErr := httpRequestValue(request)
		if requestErr != nil {
			http.Error(writer, "request rejected", http.StatusRequestEntityTooLarge)
			return
		}
		responseValue, handlerErr := invokeHTTPHandler(request.Context(), args[1], requestValue)
		if handlerErr != nil {
			http.Error(writer, "handler failed", http.StatusInternalServerError)
			return
		}
		responseStatus, responseHeaders, responseBody, responseErr := httpHandlerResponse(responseValue)
		if responseErr != nil {
			http.Error(writer, "invalid handler response", http.StatusInternalServerError)
			return
		}
		for name, values := range responseHeaders {
			for _, value := range values {
				writer.Header().Add(name, value)
			}
		}
		writer.WriteHeader(responseStatus)
		_, _ = writer.Write(responseBody)
	})
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(netListener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveErr
		return nullValue(), ctx.Err()
	case err := <-serveErr:
		if err == http.ErrServerClosed {
			return nullValue(), nil
		}
		return nullValue(), err
	}
}
