package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Dialer struct {
	Connector *Connector
	Verbose   bool

	m        sync.Mutex
	connPool []*MultiplexedConnection
	lastID   uint64
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	for c := d.takeFromPool(); c != nil; c = d.takeFromPool() {
		if conn, err := d.dialVia(ctx, c, network, address); err == nil {
			return conn, err
		}
	}

	connID := atomic.AddUint64(&d.lastID, 1)
	conn, err := d.Connector.Connect(d.makeLogger(connID))

	if err != nil {
		return nil, err
	}

	return d.dialVia(ctx, conn, network, address)
}

func (d *Dialer) dialVia(ctx context.Context, c *MultiplexedConnection, network, address string) (net.Conn, error) {
	conn, err := c.DialContext(ctx, network, address)
	if err != nil {
		c.Close()
		return nil, err
	}

	d.putToPool(c)

	return conn, err
}

func (d *Dialer) takeFromPool() *MultiplexedConnection {
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

func (d *Dialer) putToPool(c *MultiplexedConnection) {
	d.m.Lock()
	defer d.m.Unlock()

	d.connPool = append(d.connPool, c)
}

func (d *Dialer) makeLogger(connID uint64) *log.Logger {
	wr := ioutil.Discard
	if d.Verbose {
		wr = os.Stderr
	}

	return log.New(wr, fmt.Sprintf("Conn ID=%v: ", connID), 0)
}
