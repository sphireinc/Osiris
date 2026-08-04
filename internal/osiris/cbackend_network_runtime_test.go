package osiris

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeNetworkRuntimeContract(t *testing.T) {
	members := nativeNetworkRuntimeMembers()["std/net"]
	for _, member := range []string{"tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "udp_connect", "udp_send", "udp_receive", "dns_lookup"} {
		found := false
		for _, candidate := range members {
			if candidate == member {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("native network member %q is absent", member)
		}
	}
	for _, capability := range []string{"net.tcp_listen", "net.tcp_accept", "net.tcp_dial", "net.udp_bind", "net.udp_connect", "net.udp_send", "net.udp_receive", "net.dns_lookup"} {
		found := false
		for _, value := range nativeNetworkRuntimeCapabilities() {
			if value == capability {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("native network capability %q is absent", capability)
		}
	}
	for _, symbol := range []string{"onet_tcp_listen", "onet_tcp_accept", "onet_tcp_dial", "onet_read", "onet_write", "onet_udp_bind", "onet_udp_connect", "onet_udp_send", "onet_udp_receive", "onet_dns_lookup", "onet_close"} {
		if !strings.Contains(cRuntimeNetworkComponent, "static OValue "+symbol) {
			t.Fatalf("C network component is missing %s", symbol)
		}
	}
	if !strings.Contains(cRuntimeNetworkComponent, "OSIRIS_NET_MAGIC") || !strings.Contains(cRuntimeNetworkComponent, "OSIRIS_NATIVE_POSIX_NET") {
		t.Fatal("native network component is missing its ownership/platform guards")
	}
}

func TestNativeNetworkRuntimeCompilesAsAnIsolatedCComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the isolated contract uses the POSIX socket headers")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler is not installed")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "network.c")
	source := cRuntimePrelude() + cRuntimeNetworkComponent + "\nint main(void) { return 0; }\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(cc, "-std=c11", "-Werror=implicit-function-declaration", "-fsyntax-only", sourcePath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated native network component does not compile: %v\n%s", err, output)
	}
}
