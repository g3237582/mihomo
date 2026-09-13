package easytierffi

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStubDataPlane(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("stub ABI is compiled and verified on linux/amd64")
	}
	lib := buildStub(t)
	native, err := Open(lib)
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()

	if err := native.RunNetworkInstance("instance_name = \"stub\"\n"); err != nil {
		t.Fatalf("RunNetworkInstance: %v", err)
	}
	session, err := native.OpenSession("stub")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer session.Close()

	ctx := context.Background()
	conn, err := session.DialContext(ctx, "tcp", "10.77.0.2:80")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("Read = %q", buf[:n])
	}
	n, err = conn.Write([]byte("world"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 5 {
		t.Fatalf("Write n = %d", n)
	}

	pc, err := session.ListenPacket(0)
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer pc.Close()
	if _, err := pc.WriteTo([]byte("ping"), &net.UDPAddr{IP: net.ParseIP("10.77.0.2"), Port: 53}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	n, addr, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if string(buf[:n]) != "pong" {
		t.Fatalf("ReadFrom = %q", buf[:n])
	}
	if addr == nil || addr.(*net.UDPAddr).Port != 53 {
		t.Fatalf("unexpected udp peer %v", addr)
	}
}

func buildStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "libeasytier_ffi_stub.so")
	src := filepath.Join("testdata", "stub.c")
	cmd := exec.Command("gcc", "-shared", "-fPIC", "-O1", "-o", out, src)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("compile stub: %v", err)
	}
	return out
}
