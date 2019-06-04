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
	connPool []*connWithId
	lastId   uint64
}

type connWithId struct {
	id     uint64
	mc     *MultiplexedConnection
	logger *log.Logger
}

func newConnWithId(id uint64, verbose bool) *connWithId {
	wr := ioutil.Discard
	if verbose {
		wr = os.Stderr
	}

	return &connWithId{
		id:     id,
		logger: log.New(wr, fmt.Sprintf("Connection %v: ", id), 0),
	}
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	for c := d.takeFromPool(); c != nil; c = d.takeFromPool() {
		if conn, err := d.dialVia(ctx, c, network, address); err == nil {
			return conn, err
		}
	}

	connId := atomic.AddUint64(&d.lastId, 1)
	cwi := newConnWithId(connId, d.Verbose)
	cwi.logger.Print("connecting to upstream")

	mc, err := d.Connector.Connect()

	if err != nil {
		cwi.logger.Printf("error while connecting to upstream: %v", err)
		return nil, err
	}
	cwi.mc = mc
	mc.conn.(*connectionWrapper).logger = cwi.logger
	return d.dialVia(ctx, cwi, network, address)
}

func (d *Dialer) dialVia(ctx context.Context, c *connWithId, network, address string) (net.Conn, error) {
	logger := log.New(c.logger.Writer(),
		fmt.Sprintf("%sproxy to %s://%s: ", c.logger.Prefix(), network, address),
		0)

	logger.Print("dialing")
	conn, err := c.mc.DialContext(ctx, network, address)
	if err != nil {
		logger.Printf("error while dialing: %v", err)
		c.mc.Close()
		return nil, err
	}

	logger.Print("connected")
	d.putToPool(c)

	conn.(*connectionWrapper).logger = logger
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
