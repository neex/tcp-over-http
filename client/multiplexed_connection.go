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
	config  *MultiplexedConnectionConfig
	session *yamux.Session

	m                  sync.Mutex
	dialable, closed   bool
	cntActive, cntUsed int
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
		config:   config,
		session:  session,
		dialable: true,
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
	c.dialable = false
	c.checkClose()
}

func (c *MultiplexedConnection) IsDialable() bool {
	c.m.Lock()
	defer c.m.Unlock()

	if c.session.IsClosed() {
		c.dialable = false
		c.checkClose()
	}

	if !c.dialable {
		return false
	}

	return true
}

func (c *MultiplexedConnection) Ping() (time.Duration, error) {
	return c.session.Ping()
}

func (c *MultiplexedConnection) registerConnect() int {
	c.m.Lock()
	defer c.m.Unlock()

	if !c.dialable {
		return 0
	}

	max := c.config.MaxMultiplexedConnections

	c.cntUsed++
	c.cntActive++
	if max > 0 && c.cntUsed >= max {
		c.dialable = false
	}
	return c.cntUsed
}

func (c *MultiplexedConnection) registerDisconnect() {
	c.m.Lock()
	defer c.m.Unlock()
	c.cntActive--
	c.checkClose()
}

func (c *MultiplexedConnection) checkClose() {
	needClose := c.cntActive == 0 && !c.dialable && !c.closed
	if needClose {
		go func() {
			c.config.Logger.Debug("closing session")
			_ = c.session.Close()
		}()
		c.closed = true
	}
}
