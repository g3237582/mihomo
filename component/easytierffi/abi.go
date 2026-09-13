// Package easytierffi binds libeasytier_ffi through purego (no cgo).
//
// The data-plane ABI is the current session + submit/wait/take surface
// (EasyTier ffi-dataplane). A 20-byte DataPlaneSocketAddr is passed by value
// in C; on SysV/Apple AMD64 and AAPCS64 that means the caller passes a
// pointer. This package relies on that MEMORY-class passing. linux/amd64 is
// the verified target.
package easytierffi

import (
	"fmt"
	"net"
	"net/netip"
)

// SocketAddr is the EasyTier DataPlaneSocketAddr C layout.
// Family 4 is IPv4; ABI v3 rejects IPv6.
type SocketAddr struct {
	Family  uint16
	Port    uint16
	Address [16]byte
}

// Completion is the EasyTier DataPlaneCompletion C layout (16 bytes).
type Completion struct {
	OperationID   uint64
	OperationKind uint16
	Status        uint16
}

const (
	familyIPv4 = 4

	deadlineRead  = 1 << 0
	deadlineWrite = 1 << 1
)

func ipv4SocketAddr(ip net.IP, port uint16) (SocketAddr, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return SocketAddr{}, fmt.Errorf("easytier data plane ABI v3 supports IPv4 only, got %s", ip)
	}
	var addr SocketAddr
	addr.Family = familyIPv4
	addr.Port = port
	copy(addr.Address[:4], ip4)
	return addr, nil
}

func (a SocketAddr) ip() net.IP {
	switch a.Family {
	case familyIPv4:
		return net.IPv4(a.Address[0], a.Address[1], a.Address[2], a.Address[3]).To4()
	default:
		return nil
	}
}

func (a SocketAddr) TCPAddr() *net.TCPAddr {
	return &net.TCPAddr{IP: a.ip(), Port: int(a.Port)}
}

func (a SocketAddr) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{IP: a.ip(), Port: int(a.Port)}
}

func parseIPPort(address string) (net.IP, uint16, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, 0, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		if parsed, parseErr := netip.ParseAddr(host); parseErr == nil {
			ip = parsed.AsSlice()
		}
	}
	if ip == nil {
		return nil, 0, fmt.Errorf("easytier data plane requires an IP address, got %q", host)
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return nil, 0, err
	}
	if port < 0 || port > 65535 {
		return nil, 0, fmt.Errorf("invalid port %d", port)
	}
	return ip, uint16(port), nil
}
