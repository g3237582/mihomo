package easytierffi

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Native is a loaded libeasytier_ffi handle plus bound C symbols.
type Native struct {
	lib uintptr

	runNetworkInstance     func(*byte) int32
	retainNetworkInstance  func(**byte, uintptr) int32
	deleteNetworkInstance  func(**byte, uintptr) int32
	getErrorMsg            func(**byte)
	freeString             func(*byte)
	sessionOpen            func(*byte, *uint64) int32
	sessionClose           func(uint64) int32
	tcpConnectSubmit       func(uint64, *SocketAddr, uint64, *uint64) int32
	tcpConnectTake         func(uint64, uint64, *uint64, *SocketAddr, *SocketAddr) int32
	tcpReadSubmit          func(uint64, uint64, uint32, *uint64) int32
	tcpReadTake            func(uint64, uint64, *byte, uint32, *uint32, *bool) int32
	tcpWriteSubmit         func(uint64, uint64, *byte, uint32, *uint64) int32
	tcpWriteTake           func(uint64, uint64, *uint32) int32
	udpBindSubmit          func(uint64, uint16, uint64, *uint64) int32
	udpBindTake            func(uint64, uint64, *uint64, *SocketAddr) int32
	udpSendSubmit          func(uint64, uint64, *SocketAddr, *byte, uint32, *uint64) int32
	udpSendTake            func(uint64, uint64, *uint32) int32
	udpReceiveSubmit       func(uint64, uint64, uint32, *uint64) int32
	udpReceiveTake         func(uint64, uint64, *byte, uint32, *uint32, *SocketAddr, *bool) int32
	completionWait         func(uint64, uint64) int32
	completionDrain        func(uint64, *Completion, uint32) int32
	operationCancel        func(uint64, uint64) int32
	operationFree          func(uint64, uint64) int32
	resourceClose          func(uint64, uint64) int32
	resourceDeadlineSet    func(uint64, uint64, uint32, uint64) int32
}

// Open loads libeasytier_ffi from path. The library must be built with
// features c-abi and ffi-dataplane (the crate defaults).
func Open(path string) (*Native, error) {
	if path == "" {
		return nil, errors.New("easytier ffi-library path is empty")
	}
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load easytier ffi library %q: %w", path, err)
	}
	n := &Native{lib: lib}
	if err := n.bind(); err != nil {
		_ = purego.Dlclose(lib)
		return nil, err
	}
	return n, nil
}

func (n *Native) Close() error {
	if n == nil || n.lib == 0 {
		return nil
	}
	err := purego.Dlclose(n.lib)
	n.lib = 0
	return err
}

func (n *Native) bind() error {
	type binding struct {
		fn   any
		name string
	}
	regs := []binding{
		{&n.runNetworkInstance, "run_network_instance"},
		{&n.retainNetworkInstance, "retain_network_instance"},
		{&n.deleteNetworkInstance, "delete_network_instance"},
		{&n.getErrorMsg, "get_error_msg"},
		{&n.freeString, "free_string"},
		{&n.sessionOpen, "data_plane_session_open"},
		{&n.sessionClose, "data_plane_session_close"},
		{&n.tcpConnectSubmit, "data_plane_tcp_connect_submit"},
		{&n.tcpConnectTake, "data_plane_tcp_connect_result_take"},
		{&n.tcpReadSubmit, "data_plane_tcp_read_submit"},
		{&n.tcpReadTake, "data_plane_tcp_read_result_take"},
		{&n.tcpWriteSubmit, "data_plane_tcp_write_submit"},
		{&n.tcpWriteTake, "data_plane_tcp_write_result_take"},
		{&n.udpBindSubmit, "data_plane_udp_bind_submit"},
		{&n.udpBindTake, "data_plane_udp_bind_result_take"},
		{&n.udpSendSubmit, "data_plane_udp_send_submit"},
		{&n.udpSendTake, "data_plane_udp_send_result_take"},
		{&n.udpReceiveSubmit, "data_plane_udp_receive_submit"},
		{&n.udpReceiveTake, "data_plane_udp_receive_result_take"},
		{&n.completionWait, "data_plane_completion_wait"},
		{&n.completionDrain, "data_plane_completion_drain"},
		{&n.operationCancel, "data_plane_operation_cancel"},
		{&n.operationFree, "data_plane_operation_free"},
		{&n.resourceClose, "data_plane_resource_close"},
		{&n.resourceDeadlineSet, "data_plane_resource_deadline_set"},
	}
	for _, b := range regs {
		if err := registerSym(n.lib, b.fn, b.name); err != nil {
			return err
		}
	}
	return nil
}

func registerSym(lib uintptr, fnptr any, name string) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("bind %s: %v", name, rec)
		}
	}()
	purego.RegisterLibFunc(fnptr, lib, name)
	return nil
}

func (n *Native) RunNetworkInstance(config string) error {
	defer pinErrorThread()()
	cfg := cString(config)
	if n.runNetworkInstance(&cfg[0]) != 0 {
		return n.lastError()
	}
	runtime.KeepAlive(cfg)
	return nil
}

func (n *Native) RetainNetworkInstances(instances []string) error {
	return n.withNameList(n.retainNetworkInstance, instances)
}

func (n *Native) DeleteNetworkInstances(instances []string) error {
	return n.withNameList(n.deleteNetworkInstance, instances)
}

func (n *Native) withNameList(fn func(**byte, uintptr) int32, instances []string) error {
	defer pinErrorThread()()
	if len(instances) == 0 {
		if fn(nil, 0) != 0 {
			return n.lastError()
		}
		return nil
	}
	cStrings := make([][]byte, len(instances))
	ptrs := make([]*byte, len(instances))
	for i, name := range instances {
		cStrings[i] = cString(name)
		ptrs[i] = &cStrings[i][0]
	}
	if fn(&ptrs[0], uintptr(len(instances))) != 0 {
		return n.lastError()
	}
	runtime.KeepAlive(cStrings)
	runtime.KeepAlive(ptrs)
	return nil
}

func (n *Native) OpenSession(instance string) (*Session, error) {
	defer pinErrorThread()()
	name := cString(instance)
	var handle uint64
	if n.sessionOpen(&name[0], &handle) != 0 {
		return nil, n.lastError()
	}
	runtime.KeepAlive(name)
	if handle == 0 {
		return nil, errors.New("easytier data-plane session handle is zero")
	}
	s := newSession(n, handle)
	s.start()
	return s, nil
}

func (n *Native) lastError() error {
	var out *byte
	n.getErrorMsg(&out)
	if out == nil {
		return errors.New("easytier ffi call failed")
	}
	msg := readCString(out)
	n.freeString(out)
	if strings.Contains(strings.ToLower(msg), "timed out") || strings.Contains(strings.ToLower(msg), "timeout") {
		return timeoutError(msg)
	}
	return errors.New(msg)
}

func (n *Native) statusError(status uint16) error {
	if err := n.lastError(); err != nil && err.Error() != "easytier ffi call failed" {
		return err
	}
	return fmt.Errorf("easytier data-plane operation failed (status %d)", status)
}

func pinErrorThread() func() {
	runtime.LockOSThread()
	return runtime.UnlockOSThread
}

func cString(s string) []byte {
	if strings.ContainsRune(s, 0) {
		panic("easytier ffi string contains NUL")
	}
	return append([]byte(s), 0)
}

func readCString(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var n int
	for *(*byte)(unsafe.Add(unsafe.Pointer(ptr), n)) != 0 {
		n++
	}
	return unsafe.String(ptr, n)
}

type timeoutError string

func (e timeoutError) Error() string   { return string(e) }
func (e timeoutError) Timeout() bool   { return true }
func (e timeoutError) Temporary() bool { return true }
