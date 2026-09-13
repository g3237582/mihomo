package easytierffi

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const (
	defaultTimeout   = 30 * time.Second
	waitSliceMS      = 50
	maxDrainPerWait  = 32
)

var (
	errSessionClosed = errors.New("easytier data-plane session closed")
	errEmptyBuffer   = errors.New("empty buffer")
)

// Session is one native data-plane session. EasyTier allows only one open
// session per instance, so all TCP/UDP handles for that instance share it.
type Session struct {
	n      *Native
	id     uint64
	mu     sync.Mutex
	waiters map[uint64]chan Completion
	pending map[uint64]Completion
	stop   chan struct{}
	done   chan struct{}
	closed bool
}

func newSession(n *Native, id uint64) *Session {
	return &Session{
		n:       n,
		id:      id,
		waiters: make(map[uint64]chan Completion),
		pending: make(map[uint64]Completion),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (s *Session) start() {
	go s.loop()
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.stop)
	s.mu.Unlock()
	<-s.done
	defer pinErrorThread()()
	if s.n.sessionClose(s.id) != 0 {
		return s.n.lastError()
	}
	return nil
}

func (s *Session) loop() {
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			s.failWaiters(errSessionClosed)
			return
		default:
		}
		r := s.n.completionWait(s.id, waitSliceMS)
		if r < 0 {
			select {
			case <-s.stop:
				s.failWaiters(errSessionClosed)
				return
			default:
				// Transient wait error; retry until Close.
				continue
			}
		}
		if r == 0 {
			continue
		}
		var buf [maxDrainPerWait]Completion
		n := s.n.completionDrain(s.id, &buf[0], uint32(len(buf)))
		if n < 0 {
			continue
		}
		for i := 0; i < int(n) && i < len(buf); i++ {
			s.deliver(buf[i])
		}
	}
}

func (s *Session) deliver(c Completion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.waiters[c.OperationID]; ok {
		delete(s.waiters, c.OperationID)
		ch <- c
		return
	}
	s.pending[c.OperationID] = c
}

func (s *Session) failWaiters(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.waiters {
		close(ch)
		delete(s.waiters, id)
	}
	s.pending = map[uint64]Completion{}
	_ = err
}

func (s *Session) register(op uint64) (chan Completion, Completion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, Completion{}, false
	}
	if c, ok := s.pending[op]; ok {
		delete(s.pending, op)
		return nil, c, true
	}
	ch := make(chan Completion, 1)
	s.waiters[op] = ch
	return ch, Completion{}, false
}

func (s *Session) unregister(op uint64) {
	s.mu.Lock()
	delete(s.waiters, op)
	delete(s.pending, op)
	s.mu.Unlock()
}

func (s *Session) waitOp(ctx context.Context, op uint64) (Completion, error) {
	ch, cached, hasCached := s.register(op)
	if hasCached {
		if cached.Status != 0 {
			return cached, s.n.statusError(cached.Status)
		}
		return cached, nil
	}
	if ch == nil {
		return Completion{}, errSessionClosed
	}
	select {
	case <-ctx.Done():
		s.n.operationCancel(s.id, op)
		s.unregister(op)
		return Completion{}, ctx.Err()
	case c, open := <-ch:
		if !open {
			return Completion{}, errSessionClosed
		}
		if c.Status != 0 {
			return c, s.n.statusError(c.Status)
		}
		return c, nil
	}
}

func (s *Session) submitAndWait(ctx context.Context, submit func(*uint64) int32) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	defer pinErrorThread()()
	var op uint64
	if submit(&op) != 0 {
		return 0, s.n.lastError()
	}
	if op == 0 {
		return 0, errors.New("easytier data-plane operation handle is zero")
	}
	if _, err := s.waitOp(ctx, op); err != nil {
		s.n.operationFree(s.id, op)
		return 0, err
	}
	return op, nil
}

func (s *Session) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, net.UnknownNetworkError(network)
	}
	ip, port, err := parseIPPort(address)
	if err != nil {
		return nil, err
	}
	if ip.To4() == nil {
		return nil, errors.New("easytier data plane ABI v3 supports IPv4 only")
	}
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	addr, err := ipv4SocketAddr(ip, port)
	if err != nil {
		return nil, err
	}
	timeoutMS := timeoutMillis(ctx)
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.callTCPConnectSubmit(s.id, addr, timeoutMS, out)
	})
	if err != nil {
		return nil, err
	}
	defer pinErrorThread()()
	var stream uint64
	var local, peer SocketAddr
	if s.n.tcpConnectTake(s.id, op, &stream, &local, &peer) != 0 {
		s.n.operationFree(s.id, op)
		return nil, s.n.lastError()
	}
	if stream == 0 {
		return nil, errors.New("easytier tcp connect returned a zero stream")
	}
	return newConn(s, stream, local.TCPAddr(), peer.TCPAddr()), nil
}

func (s *Session) ListenPacket(localPort uint16) (net.PacketConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.udpBindSubmit(s.id, localPort, timeoutMillis(ctx), out)
	})
	if err != nil {
		return nil, err
	}
	defer pinErrorThread()()
	var socket uint64
	var local SocketAddr
	if s.n.udpBindTake(s.id, op, &socket, &local) != 0 {
		s.n.operationFree(s.id, op)
		return nil, s.n.lastError()
	}
	if socket == 0 {
		return nil, errors.New("easytier udp bind returned a zero socket")
	}
	return newPacketConn(s, socket, local.UDPAddr()), nil
}

func (s *Session) tcpRead(ctx context.Context, stream uint64, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.tcpReadSubmit(s.id, stream, uint32(len(buf)), out)
	})
	if err != nil {
		return 0, err
	}
	defer pinErrorThread()()
	var n uint32
	var eof bool
	if s.n.tcpReadTake(s.id, op, &buf[0], uint32(len(buf)), &n, &eof) != 0 {
		s.n.operationFree(s.id, op)
		return 0, s.n.lastError()
	}
	if eof && n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (s *Session) tcpWrite(ctx context.Context, stream uint64, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.tcpWriteSubmit(s.id, stream, &buf[0], uint32(len(buf)), out)
	})
	if err != nil {
		return 0, err
	}
	defer pinErrorThread()()
	var n uint32
	if s.n.tcpWriteTake(s.id, op, &n) != 0 {
		s.n.operationFree(s.id, op)
		return 0, s.n.lastError()
	}
	return int(n), nil
}

func (s *Session) udpSend(ctx context.Context, socket uint64, addr SocketAddr, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.callUDPSendSubmit(s.id, socket, addr, &buf[0], uint32(len(buf)), out)
	})
	if err != nil {
		return 0, err
	}
	defer pinErrorThread()()
	var n uint32
	if s.n.udpSendTake(s.id, op, &n) != 0 {
		s.n.operationFree(s.id, op)
		return 0, s.n.lastError()
	}
	return int(n), nil
}

func (s *Session) udpReceive(ctx context.Context, socket uint64, buf []byte) (int, *net.UDPAddr, error) {
	if len(buf) == 0 {
		return 0, nil, errEmptyBuffer
	}
	op, err := s.submitAndWait(ctx, func(out *uint64) int32 {
		return s.n.udpReceiveSubmit(s.id, socket, uint32(len(buf)), out)
	})
	if err != nil {
		return 0, nil, err
	}
	defer pinErrorThread()()
	var n uint32
	var peer SocketAddr
	var truncated bool
	if s.n.udpReceiveTake(s.id, op, &buf[0], uint32(len(buf)), &n, &peer, &truncated) != 0 {
		s.n.operationFree(s.id, op)
		return 0, nil, s.n.lastError()
	}
	_ = truncated
	return int(n), peer.UDPAddr(), nil
}

func (s *Session) closeResource(resource uint64) error {
	defer pinErrorThread()()
	if s.n.resourceClose(s.id, resource) != 0 {
		return s.n.lastError()
	}
	return nil
}

func (s *Session) setDeadline(resource uint64, read, write bool, timeout time.Duration) error {
	var dir uint32
	if read {
		dir |= deadlineRead
	}
	if write {
		dir |= deadlineWrite
	}
	if dir == 0 {
		return nil
	}
	ms := uint64(timeout / time.Millisecond)
	if timeout <= 0 {
		ms = 1
	}
	defer pinErrorThread()()
	if s.n.resourceDeadlineSet(s.id, resource, dir, ms) != 0 {
		return s.n.lastError()
	}
	return nil
}

func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

func timeoutMillis(ctx context.Context) uint64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return uint64(defaultTimeout / time.Millisecond)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 1
	}
	return uint64(remaining / time.Millisecond)
}
