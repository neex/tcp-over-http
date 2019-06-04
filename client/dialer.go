package client

import (
	"context"
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Dialer struct {
	c *Connector

	m        sync.Mutex
	connPool []*connWithId
	lastId   uint64
}

func NewDialer(c *Connector) *Dialer {
	return &Dialer{c: c}
}

type connWithId struct {
	id uint64
	mc *MultiplexedConnection
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c := d.takeFromPool()
	if c != nil {
		if conn, err := d.dialVia(ctx, c, network, address); err == nil {
			return conn, err
		}
	}

	connId := atomic.AddUint64(&d.lastId, 1)
	log.Printf("Connection %v: connecting to upstream", connId)
	mc, err := d.c.Connect()
	if err != nil {
		log.Printf("Connection %v: error while connecting to upstream: %v", connId, err)
		return nil, err
	}

	log.Printf("Connection %v: connected to upstream", connId)
	return d.dialVia(ctx, &connWithId{id: connId, mc: mc}, network, address)
}

func (d *Dialer) dialVia(ctx context.Context, c *connWithId, network, address string) (net.Conn, error) {
	log.Printf("Connection %v: dialing to %s://%s", c.id, network, address)
	conn, err := c.mc.DialContext(ctx, network, address)
	if err != nil {
		log.Printf("Connection %v: error while dialing to %s://%s: %v", c.id, network, address, err)
		c.mc.Close()
	} else {
		log.Printf("Connection %v: connected to %s://%s", c.id, network, address)
		d.putToPool(c)
	}

	return conn, err
}

func (d *Dialer) takeFromPool() *connWithId {
	d.m.Lock()
	defer d.m.Unlock()

	n := len(d.connPool)
	if n == 0 {
		return nil
	}

	r := rand.Intn(n)
	c := d.connPool[r]
	d.connPool[r] = d.connPool[n-1]
	d.connPool = d.connPool[:n-1]
	return c
}

func (d *Dialer) putToPool(c *connWithId) {
	d.m.Lock()
	defer d.m.Unlock()

	d.connPool = append(d.connPool, c)
}
