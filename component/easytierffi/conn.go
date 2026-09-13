package easytierffi

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

type Conn struct {
	session *Session
	stream  uint64
	local   net.Addr
	remote  net.Addr
	closed  atomic.Bool
	rd      atomicDeadline
	wd      atomicDeadline
}

type PacketConn struct {
	session *Session
	socket  uint64
	local   net.Addr
	closed  atomic.Bool
	rd      atomicDeadline
	wd      atomicDeadline
}

type atomicDeadline struct{ v atomic.Int64 }

func newConn(session *Session, stream uint64, local, remote net.Addr) *Conn {
	return &Conn{session: session, stream: stream, local: local, remote: remote}
}

func newPacketConn(session *Session, socket uint64, local net.Addr) *PacketConn {
	return &PacketConn{session: session, socket: socket, local: local}
}

func (c *Conn) Read(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	n, err := c.session.tcpRead(c.rd.context(), c.stream, b)
	if err != nil {
		return 0, opError("read", c.remote, err)
	}
	return n, nil
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	n, err := c.session.tcpWrite(c.wd.context(), c.stream, b)
	if err != nil {
		return 0, opError("write", c.remote, err)
	}
	return n, nil
}

func (c *Conn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return net.ErrClosed
	}
	return c.session.closeResource(c.stream)
}

func (c *Conn) LocalAddr() net.Addr  { return c.local }
func (c *Conn) RemoteAddr() net.Addr { return c.remote }

func (c *Conn) SetDeadline(t time.Time) error {
	c.rd.set(t)
	c.wd.set(t)
	return c.session.setDeadline(c.stream, true, true, c.rd.timeout(defaultTimeout))
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.rd.set(t)
	return c.session.setDeadline(c.stream, true, false, c.rd.timeout(defaultTimeout))
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.wd.set(t)
	return c.session.setDeadline(c.stream, false, true, c.wd.timeout(defaultTimeout))
}

func (p *PacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if p.closed.Load() {
		return 0, nil, net.ErrClosed
	}
	n, addr, err := p.session.udpReceive(p.rd.context(), p.socket, b)
	if err != nil {
		return 0, nil, opError("read", nil, err)
	}
	return n, addr, nil
}

func (p *PacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if p.closed.Load() {
		return 0, net.ErrClosed
	}
	ip, port, err := parseIPPort(addr.String())
	if err != nil {
		return 0, err
	}
	peer, err := ipv4SocketAddr(ip, port)
	if err != nil {
		return 0, err
	}
	n, err := p.session.udpSend(p.wd.context(), p.socket, peer, b)
	if err != nil {
		return 0, opError("write", addr, err)
	}
	return n, nil
}

func (p *PacketConn) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return net.ErrClosed
	}
	return p.session.closeResource(p.socket)
}

func (p *PacketConn) LocalAddr() net.Addr { return p.local }

func (p *PacketConn) SetDeadline(t time.Time) error {
	p.rd.set(t)
	p.wd.set(t)
	return p.session.setDeadline(p.socket, true, true, p.rd.timeout(defaultTimeout))
}

func (p *PacketConn) SetReadDeadline(t time.Time) error {
	p.rd.set(t)
	return p.session.setDeadline(p.socket, true, false, p.rd.timeout(defaultTimeout))
}

func (p *PacketConn) SetWriteDeadline(t time.Time) error {
	p.wd.set(t)
	return p.session.setDeadline(p.socket, false, true, p.wd.timeout(defaultTimeout))
}

func (d *atomicDeadline) set(t time.Time) {
	if t.IsZero() {
		d.v.Store(0)
		return
	}
	d.v.Store(t.UnixNano())
}

func (d *atomicDeadline) timeout(fallback time.Duration) time.Duration {
	ns := d.v.Load()
	if ns == 0 {
		return fallback
	}
	remaining := time.Until(time.Unix(0, ns))
	if remaining <= 0 {
		return time.Millisecond
	}
	return remaining
}

func (d *atomicDeadline) context() context.Context {
	ns := d.v.Load()
	if ns == 0 {
		ctx, _ := context.WithTimeout(context.Background(), defaultTimeout)
		return ctx
	}
	deadline := time.Unix(0, ns)
	ctx, _ := context.WithDeadline(context.Background(), deadline)
	return ctx
}

func opError(op string, addr net.Addr, err error) error {
	return &net.OpError{Op: op, Net: "easytier", Addr: addr, Err: err}
}

var (
	_ net.Conn       = (*Conn)(nil)
	_ net.PacketConn = (*PacketConn)(nil)
)
