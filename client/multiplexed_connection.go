package client

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"

	"tcp-over-http/protocol"
)

var ErrLimitExceeded = errors.New("connection limit exceeded")

type MultiplexedConnection struct {
	m sync.Mutex

	cntConnected, cntMax, cntActive int

	closed            bool
	remoteDialTimeout time.Duration

	conn    net.Conn
	session *yamux.Session
}

func NewMultiplexedConnection(conn net.Conn, max int, remoteDialTimeout time.Duration) (*MultiplexedConnection, error) {
	session, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, err
	}

	mc := &MultiplexedConnection{
		cntMax:            max,
		remoteDialTimeout: remoteDialTimeout,
		conn:              conn,
		session:           session,
	}
	return mc, nil
}

func (c *MultiplexedConnection) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if !c.registerConnect() {
		return nil, ErrLimitExceeded
	}

	conn, err := c.session.Open()
	if err != nil {
		c.registerDisconnect()
		return nil, err
	}

	req := protocol.ConnectionRequest{
		Network: network,
		Address: address,
		Timeout: c.remoteDialTimeout,
	}

	if err := protocol.WritePacket(ctx, conn, &req); err != nil {
		c.registerDisconnect()
		return nil, err
	}

	return &connectionWrapper{Conn: conn, onDisconnect: c.registerDisconnect}, nil
}

func (c *MultiplexedConnection) Close() {
	c.m.Lock()
	defer c.m.Unlock()

	c.closed = true
	if c.cntActive == 0 {
		_ = c.session.Close()
	}
}

func (c *MultiplexedConnection) registerConnect() bool {
	c.m.Lock()
	defer c.m.Unlock()

	if c.closed {
		return false
	}

	if c.cntMax > 0 && c.cntConnected >= c.cntMax {
		return false
	}

	c.cntConnected++
	c.cntActive++
	return true
}

func (c *MultiplexedConnection) registerDisconnect() {
	c.m.Lock()
	defer c.m.Unlock()

	c.cntActive--
	if c.cntActive == 0 && c.closed {
		_ = c.session.Close()
	}
}
