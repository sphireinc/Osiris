package osiris

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
