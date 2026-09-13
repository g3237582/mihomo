package easytierffi

import (
	"net"
	"testing"
	"unsafe"
)

func TestSocketAddrLayout(t *testing.T) {
	if unsafe.Sizeof(SocketAddr{}) != 20 {
		t.Fatalf("SocketAddr size = %d, want 20", unsafe.Sizeof(SocketAddr{}))
	}
	if unsafe.Sizeof(Completion{}) != 16 {
		t.Fatalf("Completion size = %d, want 16", unsafe.Sizeof(Completion{}))
	}
}

func TestIPv4SocketAddrRoundTrip(t *testing.T) {
	addr, err := ipv4SocketAddr(net.ParseIP("10.77.0.8"), 443)
	if err != nil {
		t.Fatal(err)
	}
	if addr.Family != 4 || addr.Port != 443 {
		t.Fatalf("unexpected addr: %+v", addr)
	}
	tcp := addr.TCPAddr()
	if tcp.IP.String() != "10.77.0.8" || tcp.Port != 443 {
		t.Fatalf("round trip = %s", tcp)
	}
}

func TestIPv6Rejected(t *testing.T) {
	_, err := ipv4SocketAddr(net.ParseIP("2001:db8::1"), 80)
	if err == nil {
		t.Fatal("expected IPv6 to be rejected")
	}
}

func TestSocketAddrStackPacking(t *testing.T) {
	addr, err := ipv4SocketAddr(net.ParseIP("10.77.0.8"), 443)
	if err != nil {
		t.Fatal(err)
	}
	words := socketAddrStack(addr)
	raw := (*[20]byte)(unsafe.Pointer(&words[0]))
	if raw[0] != 4 || raw[1] != 0 {
		t.Fatalf("family bytes = %v", raw[:2])
	}
	if uint16(raw[2])|uint16(raw[3])<<8 != 443 {
		t.Fatalf("port bytes = %v", raw[2:4])
	}
	if raw[4] != 10 || raw[5] != 77 || raw[6] != 0 || raw[7] != 8 {
		t.Fatalf("ipv4 bytes = %v", raw[4:8])
	}
}

func TestParseIPPort(t *testing.T) {
	ip, port, err := parseIPPort("10.77.0.1:22")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "10.77.0.1" || port != 22 {
		t.Fatalf("got %s:%d", ip, port)
	}
	if _, _, err := parseIPPort("example.com:80"); err == nil {
		t.Fatal("hostname should be rejected")
	}
}
