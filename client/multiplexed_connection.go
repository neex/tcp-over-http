package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	log "github.com/sirupsen/logrus"

	"tcp-over-http/protocol"
)

var ErrLimitExceeded = errors.New("connection limit exceeded")

type MultiplexedConnection struct {
	m      sync.Mutex
	closed bool

	cntActive, cntUsed int
	config             *MultiplexedConnectionConfig

	conn    net.Conn
	session *yamux.Session
}

type MultiplexedConnectionConfig struct {
	MaxMultiplexedConnections int
	RemoteDialTimeout         time.Duration
	KeepAliveTimeout          time.Duration
	Logger                    *log.Entry
}

func NewMultiplexedConnection(conn net.Conn, config *MultiplexedConnectionConfig) (*MultiplexedConnection, error) {
	yamuxConfig := *yamux.DefaultConfig()
	yamuxConfig.LogOutput = config.Logger.WriterLevel(log.ErrorLevel)
	if config.KeepAliveTimeout != 0 {
		yamuxConfig.KeepAliveInterval = config.KeepAliveTimeout
		yamuxConfig.ConnectionWriteTimeout = config.KeepAliveTimeout
	}

	session, err := yamux.Client(conn, &yamuxConfig)
	if err != nil {
		config.Logger.WithError(err).Fatal("yamux.Client() failed")
		return nil, err
	}

	mc := &MultiplexedConnection{
		config:  config,
		conn:    conn,
		session: session,
	}

	return mc, nil
}

func (c *MultiplexedConnection) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	subConnID := c.registerConnect()
	if subConnID == 0 {
		return nil, ErrLimitExceeded
	}

	subConnIDStr := fmt.Sprintf("%v", subConnID)
	max := c.config.MaxMultiplexedConnections
	if max > 0 {
		subConnIDStr = fmt.Sprintf("%s/%v", subConnIDStr, max)
	}

	logger := c.config.Logger.WithFields(log.Fields{
		"subconn": subConnIDStr,
		"remote":  fmt.Sprintf("%s", address),
	})

	logger.Info("connecting")
	conn, err := c.session.Open()
	if err != nil {
		logger.WithError(err).Error("error in session.Open")
		c.registerDisconnect()
		return nil, err
	}

	req := protocol.ConnectionRequest{
		Network: network,
		Address: address,
		Timeout: c.config.RemoteDialTimeout,
	}

	if err := protocol.WritePacket(ctx, conn, &req); err != nil {
		logger.WithError(err).Error("error while writing connection request")
		c.registerDisconnect()
		return nil, err
	}

	logger.Debug("lazy connect successful")

	return &connectionWrapper{
		Conn:         conn,
		onDisconnect: c.registerDisconnect,
		logger:       logger,
	}, nil
}

func (c *MultiplexedConnection) Close() {
	c.m.Lock()
	defer c.m.Unlock()

	c.closed = true
	if c.cntActive == 0 {
		c.config.Logger.Debug("no active connections when .Close called, closing session")
		_ = c.session.Close()
	}
}

func (c *MultiplexedConnection) registerConnect() int {
	c.m.Lock()
	defer c.m.Unlock()

	if c.closed {
		return 0
	}

	max := c.config.MaxMultiplexedConnections
	if max > 0 && c.cntUsed >= max {
		return 0
	}

	c.cntUsed++
	c.cntActive++
	return c.cntUsed
}

func (c *MultiplexedConnection) registerDisconnect() {
	c.m.Lock()
	defer c.m.Unlock()

	c.cntActive--
	if c.cntActive == 0 && c.closed {
		c.config.Logger.Debug("no active connections left, closing session")
		_ = c.session.Close()
	}
}
