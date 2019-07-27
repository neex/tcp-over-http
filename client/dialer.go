package client

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/neex/tcp-over-http/protocol"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Dialer struct {
	Connector          *Connector
	PreconnectPoolSize int

	m        sync.Mutex
	closed   bool
	connPool []*MultiplexedConnection

	lastID uint64

	preconnectOnce sync.Once
	prevPoolSize   int
}

func (d *Dialer) Close() {
	d.m.Lock()
	defer d.m.Unlock()
	for _, mc := range d.connPool {
		mc.Close()
	}
	d.closed = true
}

func (d *Dialer) Closed() bool {
	d.m.Lock()
	defer d.m.Unlock()

	return d.closed
}

func (d *Dialer) EnablePreconnect() {
	d.preconnectOnce.Do(func() {
		go func() {
			if d.PreconnectPoolSize == 0 {
				return
			}
			d.prevPoolSize = d.PreconnectPoolSize

			for {
				if d.Closed() {
					d.Close()
					return
				}

				if !d.refillPreconnectPool() {
					time.Sleep(time.Second)
				}
			}
		}()
	})
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	maybeWrap := func(c net.Conn) net.Conn {
		if c != nil && (network == "udp" || network == "udp4" || network == "udp6") {
			return protocol.NewPacketConnection(c)
		}
		return c
	}
	if c := d.takeFromPool(); c != nil {
		if conn, err := d.dialVia(ctx, c, network, address); err == nil {
			return maybeWrap(conn), err
		}
	}

	mc, err := d.makeConn()

	if err != nil {
		return nil, err
	}

	conn, err := d.dialVia(ctx, mc, network, address)
	return maybeWrap(conn), err
}

func (d *Dialer) Ping() (time.Duration, error) {
	if c := d.takeFromPool(); c != nil {
		defer func() {
			if c.IsDialable() {
				d.putToPool(c)
			}
		}()
		return c.Ping()
	}
	return 0, errors.New("no active connections")
}

func (d *Dialer) dialVia(ctx context.Context, c *MultiplexedConnection, network, address string) (net.Conn, error) {
	conn, err := c.DialContext(ctx, network, address)
	if err != nil {
		c.Close()
		return nil, err
	}

	if c.IsDialable() {
		d.putToPool(c)
	}

	return conn, err
}

func (d *Dialer) refillPreconnectPool() bool {
	d.m.Lock()
	cnt := 0
	for i := range d.connPool {
		if d.connPool[i].IsDialable() {
			d.connPool[cnt] = d.connPool[i]
			cnt++
		}
	}
	d.connPool = d.connPool[:cnt]
	d.m.Unlock()

	max := cnt
	if cnt < d.prevPoolSize {
		max = d.prevPoolSize
	}
	logger := log.WithField("seen_pool_size", max)

	if max >= d.PreconnectPoolSize {
		d.prevPoolSize = cnt
		logger.Trace("preconnect pool full")
		return false
	}

	logger.Info("preconnect triggered")

	conn, err := d.makeConn()
	if err != nil {
		log.WithError(err).Error("filling upstream connection pool")
		return false
	}

	d.putToPool(conn)
	return true
}

func (d *Dialer) makeConn() (*MultiplexedConnection, error) {
	connID := atomic.AddUint64(&d.lastID, 1)
	return d.Connector.Connect(log.WithField("upstream_conn", connID))
}

func (d *Dialer) takeFromPool() *MultiplexedConnection {
	d.m.Lock()
	defer d.m.Unlock()

	for {
		n := len(d.connPool)
		if n == 0 {
			return nil
		}

		r := rand.Intn(n)
		c := d.connPool[r]
		d.connPool[r] = d.connPool[n-1]
		d.connPool = d.connPool[:n-1]

		if c.IsDialable() {
			return c
		}
	}
}

func (d *Dialer) putToPool(c *MultiplexedConnection) {
	d.m.Lock()
	defer d.m.Unlock()

	d.connPool = append(d.connPool, c)
}
