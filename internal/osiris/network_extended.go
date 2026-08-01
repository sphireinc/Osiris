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
		if err != nil || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") {
			return nil, fmt.Errorf("invalid HTTP header name")
		}
		value, err := entry.value.AsString()
		if err != nil || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("invalid HTTP header value")
		}
		header.Set(name, value)
	}
	return header, nil
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

type boundedResponseBody struct {
	reader io.Reader
	closer io.Closer
	read   int64
}

func (body *boundedResponseBody) Read(buffer []byte) (int, error) {
	if body.read >= maxHTTPBodyBytes {
		return 0, fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
	}
	remaining := maxHTTPBodyBytes - body.read
	if int64(len(buffer)) > remaining+1 {
		buffer = buffer[:remaining+1]
	}
	n, err := body.reader.Read(buffer)
	body.read += int64(n)
	if body.read > maxHTTPBodyBytes {
		return n, fmt.Errorf("HTTP response body exceeds %d bytes", maxHTTPBodyBytes)
	}
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
			requestBody = streamBodyReader{stream: args[3].stream}
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

// httpServeContextValue is intentionally a bounded static responder. Dynamic
// handler callbacks require task/closure lifetime rules and remain a separate
// API; this primitive is enough for local health endpoints and fixtures while
// retaining cancellation and graceful listener shutdown.
func httpServeContextValue(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return nullValue(), fmt.Errorf("http_serve expects listener, response body, and optional status")
	}
	listener, err := args[0].AsTCPListener()
	if err != nil {
		return nullValue(), err
	}
	body, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	status := 200
	if len(args) == 3 {
		status, err = args[2].Int()
		if err != nil || status < 100 || status > 599 {
			return nullValue(), fmt.Errorf("http_serve status must be between 100 and 599")
		}
	}
	listener.mu.Lock()
	netListener := listener.listener
	listener.mu.Unlock()
	if netListener == nil {
		return nullValue(), fmt.Errorf("tcp listener is closed")
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, body)
	})}
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
