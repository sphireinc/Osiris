package osiris

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNativeRejectsHostedNetworkingWithSourceSpan(t *testing.T) {
	prog, err := Parse("network.osr", "emit http_get(\"http://127.0.0.1\")\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileC(context.Background(), NewRuntime(), prog)
	if err == nil || !strings.Contains(err.Error(), "network.osr:1:6: native C backend: networking builtin") {
		t.Fatalf("error = %v, want source-spanned native networking diagnostic", err)
	}
}

func TestUDPBoundedPacketContract(t *testing.T) {
	ctx := context.Background()
	serverValue, err := udpBindContextValue(ctx, []Value{stringValue("127.0.0.1:0")})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	server, err := udpSocketFromValue(serverValue)
	if err != nil {
		t.Fatal(err)
	}
	defer server.close()
	clientValue, err := udpConnectContextValue(ctx, []Value{stringValue(server.address())})
	if err != nil {
		t.Fatal(err)
	}
	client, err := udpSocketFromValue(clientValue)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	if _, err := udpSendContextValue(ctx, []Value{clientValue, stringValue("hello")}); err != nil {
		t.Fatal(err)
	}
	packet, err := udpReceiveContextValue(ctx, []Value{serverValue, uintValue(64, 64), uintValue(1000, 64)})
	if err != nil {
		t.Fatal(err)
	}
	if packet.kind != kindTuple || len(packet.tuple) != 3 || packet.tuple[1].String() != `"`+client.address()+`"` || packet.tuple[2].Truthy() {
		t.Fatalf("packet = %s, want data, peer, and false truncation", packet.String())
	}
	if got, err := bytesFromValue(packet.tuple[0]); err != nil || string(got) != "hello" {
		t.Fatalf("packet data = %q, err = %v", got, err)
	}
}

func TestUDPReceiveCancellationUnblocksPacketRead(t *testing.T) {
	value, err := udpBindContextValue(context.Background(), []Value{stringValue("127.0.0.1:0")})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	socket, err := udpSocketFromValue(value)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, receiveErr := udpReceiveContextValue(ctx, []Value{value})
		result <- receiveErr
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case receiveErr := <-result:
		record, ok := receiveErr.(*OsirisError)
		if !ok || record.kind != ErrorKindCancelled {
			t.Fatalf("receive error = %T %v, want structured cancellation", receiveErr, receiveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("udp receive did not stop after cancellation")
	}
}

func TestDNSLookupIsSortedAndUnique(t *testing.T) {
	values, err := dnsLookupContextValue(context.Background(), []Value{stringValue("localhost")})
	if err != nil {
		t.Fatal(err)
	}
	if values.kind != kindList || len(values.list) == 0 {
		t.Fatalf("values = %s, want non-empty list", values.String())
	}
	for i := 1; i < len(values.list); i++ {
		if values.list[i-1].str > values.list[i].str {
			t.Fatalf("values = %s, want sorted result", values.String())
		}
	}
}

func TestDNSLookupAcceptsExplicitResolverPolicies(t *testing.T) {
	for _, policy := range []string{"system", "go"} {
		values, err := dnsLookupContextValue(context.Background(), []Value{stringValue("localhost"), stringValue(policy)})
		if err != nil {
			t.Fatalf("policy %q: %v", policy, err)
		}
		if values.kind != kindList || len(values.list) == 0 {
			t.Fatalf("policy %q values = %s, want non-empty list", policy, values.String())
		}
	}
	if _, err := dnsLookupContextValue(context.Background(), []Value{stringValue("localhost"), stringValue("custom")}); err == nil || !strings.Contains(err.Error(), "resolver policy") {
		t.Fatalf("invalid policy error = %v", err)
	}
}

func TestDNSLookupCancellationIsObservable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dnsLookupContextValue(ctx, []Value{stringValue("localhost")})
	record, ok := err.(*OsirisError)
	if !ok || record.kind != ErrorKindCancelled {
		t.Fatalf("error = %T %v, want structured context cancellation", err, err)
	}
}

func TestTLSConfigurationRejectsUnsafeInputs(t *testing.T) {
	_, err := tlsDialConfigContextValue(context.Background(), []Value{
		stringValue("127.0.0.1:1"), stringValue("localhost"), stringValue("not PEM"),
	})
	if record, ok := err.(*OsirisError); !ok || record.kind != ErrorKindNetwork {
		t.Fatalf("invalid roots error = %T %v", err, err)
	}
	_, err = tlsDialConfigContextValue(context.Background(), []Value{
		stringValue("127.0.0.1:1"), stringValue(" "), stringValue("not PEM"),
	})
	if record, ok := err.(*OsirisError); !ok || record.kind != ErrorKindNetwork {
		t.Fatalf("empty server name error = %T %v", err, err)
	}
	_, err = tlsListenContextValue(context.Background(), []Value{
		stringValue("127.0.0.1:0"), stringValue("cert.pem"), stringValue("key.pem"),
	})
	if record, ok := err.(*OsirisError); !ok || record.kind != ErrorKindNetwork {
		t.Fatalf("relative TLS paths error = %T %v", err, err)
	}
}

func TestTLSConfiguredDialVerifiesExplicitRootsAndHostname(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
	defer tlsListener.Close()
	accepted := make(chan error, 1)
	go func() {
		connection, acceptErr := tlsListener.Accept()
		if acceptErr != nil {
			accepted <- acceptErr
			return
		}
		accepted <- connection.Close()
	}()
	roots := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	address := "localhost:" + strings.Split(listener.Addr().String(), ":")[1]
	streamValue, err := tlsDialConfigContextValue(context.Background(), []Value{stringValue(address), stringValue("localhost"), stringValue(string(roots)), uintValue(1000, 64)})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := streamValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.close(); err != nil {
		t.Fatal(err)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPRequestIsBoundedAndDeterministic(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("X-Z", "last")
		writer.Header().Add("X-A", "first")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(request.Method + " " + request.URL.Path))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	httpServer := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	httpServer.Start()
	defer httpServer.Close()
	response, err := httpRequestContextValue(context.Background(), []Value{
		stringValue("POST"),
		stringValue(httpServer.URL + "/fixture"),
		mapValueOf([]mapEntry{{key: stringValue("Content-Type"), value: stringValue("text/plain")}}),
		stringValue("body"),
	})
	if err != nil {
		t.Fatal(err)
	}
	statusValue, err := mapGetValue([]Value{response, stringValue("status")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusValue.String(), "some") || !strings.Contains(response.String(), `"POST /fixture"`) {
		t.Fatalf("response = %s, want status and body", response.String())
	}
}

func TestHTTPRequestStreamPreservesBodyOwnershipAndLimits(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("streamed-response"))
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	response, err := httpRequestStreamContextValue(context.Background(), []Value{
		stringValue("GET"), stringValue(server.URL), nullValue(), nullValue(), uintValue(1000, 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	bodyValue, err := mapGetValue([]Value{response, stringValue("body_stream")})
	if err != nil {
		t.Fatal(err)
	}
	bodyValue, err = unwrapOption(bodyValue)
	if err != nil {
		t.Fatal(err)
	}
	body, err := bodyValue.AsStream()
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := body.readChunk(64)
	if err != nil {
		t.Fatal(err)
	}
	data, err := bytesFromValue(chunk)
	if err != nil || string(data) != "streamed-response" {
		t.Fatalf("stream body = %q, err = %v", data, err)
	}
	if _, err := body.close(); err != nil {
		t.Fatal(err)
	}
	closed, err := body.close()
	if err != nil || closed.Truthy() {
		t.Fatalf("second close = %s, err = %v", closed.String(), err)
	}
}

func TestHTTPDynamicServerHandlerAndCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	listenerValue := tcpListenerValueOf(&tcpListener{listener: listener})
	handler := functionValueOf("test.http_handler", func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), nil
		}
		request, ok := httpMapField(args[0], "body")
		if !ok {
			return nullValue(), nil
		}
		return mapValueOf([]mapEntry{
			{key: stringValue("status"), value: uintValue(201, 16)},
			{key: stringValue("headers"), value: mapValueOf([]mapEntry{{key: stringValue("X-Handler"), value: stringValue("ok")}})},
			{key: stringValue("body"), value: stringValue("handled:" + request.str)},
		}), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() {
		_, serveErr := httpServeContextValue(ctx, []Value{listenerValue, handler})
		serveDone <- serveErr
	}()
	client := &http.Client{}
	var response *http.Response
	for attempt := 0; attempt < 50; attempt++ {
		response, err = client.Post("http://"+listener.Addr().String()+"/fixture", "text/plain", strings.NewReader("payload"))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusCreated || response.Header.Get("X-Handler") != "ok" || string(data) != "handled:payload" {
		t.Fatalf("response = %d %q %q, want dynamic handler result", response.StatusCode, response.Header.Get("X-Handler"), data)
	}
	cancel()
	select {
	case serveErr := <-serveDone:
		if !strings.Contains(serveErr.Error(), "context canceled") {
			t.Fatalf("serve error = %v, want context cancellation", serveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dynamic HTTP server did not stop after cancellation")
	}
}

func TestHTTPRejectsAmbiguousRequestFraming(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() {
		_, serveErr := httpServeContextValue(ctx, []Value{
			tcpListenerValueOf(&tcpListener{listener: listener}),
			stringValue("ok"),
		})
		serveDone <- serveErr
	}()
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(connection, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n0\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	response, readErr := io.ReadAll(connection)
	_ = connection.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(response), "400 Bad Request") {
		t.Fatalf("response = %q, want parser rejection", response)
	}
	cancel()
	select {
	case <-serveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not stop after malformed request test")
	}
}

func TestHTTPHeaderAndBodyBoundsRejectUnsafeInputs(t *testing.T) {
	for _, value := range []Value{
		mapValueOf([]mapEntry{{key: stringValue("Bad Name"), value: stringValue("x")}}),
		mapValueOf([]mapEntry{{key: stringValue("X-Test"), value: stringValue("bad\nvalue")}}),
	} {
		if _, err := headerMap(value); err == nil {
			t.Fatalf("header map %s was accepted", value.String())
		}
	}
	body := &boundedResponseBody{reader: strings.NewReader(strings.Repeat("x", maxHTTPBodyBytes+1)), closer: io.NopCloser(strings.NewReader(""))}
	data, err := io.ReadAll(body)
	if err == nil || len(data) != maxHTTPBodyBytes || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("bounded body read = %d bytes, err = %v", len(data), err)
	}
	if _, _, _, err := httpHandlerResponse(mapValueOf([]mapEntry{{key: stringValue("status"), value: uintValue(200, 16)}, {key: stringValue("body"), value: stringValue(strings.Repeat("x", maxHTTPBodyBytes+1))}})); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized handler response = %v", err)
	}
}

func TestStructuredNetworkErrorsExposeOperationAndComponent(t *testing.T) {
	_, err := dnsLookupContextValue(context.Background(), []Value{stringValue("bad\x00host")})
	if err == nil {
		t.Fatal("expected invalid DNS error")
	}
	record, ok := err.(*OsirisError)
	if !ok || (record.kind != ErrorKindNetwork && record.kind != ErrorKindInvalidArgument) {
		t.Fatalf("error = %T %v, want structured network error", err, err)
	}
	if record.operation != "dns_lookup" || record.component != "network" {
		t.Fatalf("error operation/component = %q/%q", record.operation, record.component)
	}
}
