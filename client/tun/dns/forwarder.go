package dns

import (
	"context"
	"io"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/neex/tcp-over-http/common"
	"github.com/neex/tcp-over-http/protocol"
)

type ReplyHandler func(request []byte, reply []byte)

var newDNSTCPConnection = protocol.NewPacketConnection

type Forwarder struct {
	m          sync.Mutex
	head, tail *request
	conn       io.ReadWriteCloser
}

func NewForwarder(dial common.DialContextFunc) *Forwarder {
	f := &Forwarder{}
	go f.resolverLoop(dial)
	return f
}

func (f *Forwarder) SendRequest(r []byte, onReply ReplyHandler) {
	f.m.Lock()
	defer f.m.Unlock()
	req := &request{r, onReply, nil}
	if f.head == nil {
		f.head = req
	} else {
		f.tail.next = req
	}
	f.tail = req
	if f.conn == nil {
		return
	}

	if _, err := f.conn.Write(r); err != nil {
		_ = f.conn.Close()
	}
}

func (f *Forwarder) resolverLoop(dial common.DialContextFunc) {
	for {
		rawConn, err := dial(context.Background(), "tcp", "1.1.1.1:53")
		if err != nil {
			log.WithError(err).Error("cannot connect to dns")
			time.Sleep(1 * time.Second)
			continue
		}

		conn := newDNSTCPConnection(rawConn)
		if err := f.resetConnection(conn); err != nil {
			log.WithError(err).Error("cannot write requests from queue")
			time.Sleep(1 * time.Second)
			continue
		}

		f.handleConnection()
	}
}

func (f *Forwarder) resetConnection(rwc io.ReadWriteCloser) error {
	f.m.Lock()
	defer f.m.Unlock()

	if rwc == nil {
		f.conn = nil
		return nil
	}

	if f.head == nil {
		r := &request{
			request: nullReq,
		}
		f.head = r
		f.tail = r
	}

	for c := f.head; c != nil; c = c.next {
		if _, err := rwc.Write(c.request); err != nil {
			return err
		}
	}

	f.conn = rwc

	return nil
}

func (f *Forwarder) handleConnection() {
	c := f.conn
	for {
		packet := make([]byte, 65536)
		n, err := c.Read(packet)
		if err != nil {
			log.WithError(err).Info("dns connection terminated")
			_ = f.resetConnection(nil)
			_ = c.Close()
			return
		}
		reply := packet[:n]
		f.handleReply(reply)
	}
}

func (f *Forwarder) handleReply(reply []byte) {
	f.m.Lock()
	defer f.m.Unlock()
	r := f.head
	if r == nil {
		log.Error("received dns reply when no request in queue")
		return
	}

	f.head = r.next
	if f.head == nil {
		f.tail = nil
	}

	if r.onReply != nil {
		go r.onReply(r.request, reply)
	}
}

var nullReq = []byte{0x7, 0xeb, 0x1, 0x20, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0xa, 0x0, 0x1, 0x0, 0x0, 0x29, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc, 0x0, 0xa, 0x0, 0x8, 0xc4, 0x96, 0x28, 0x38, 0x6a, 0x70, 0x7d, 0x67}

type request struct {
	request []byte
	onReply ReplyHandler
	next    *request
}
