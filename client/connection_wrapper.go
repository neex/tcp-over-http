package client

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"

	"tcp-over-http/protocol"
)

type connectionWrapper struct {
	m            sync.Mutex
	responseDone bool
	onDisconnect func()
	net.Conn
}

func (cw *connectionWrapper) Read(b []byte) (n int, err error) {
	cw.ensureResponse()
	return cw.Conn.Read(b)
}

func (cw *connectionWrapper) Close() error {
	defer cw.onDisconnect()
	return cw.Conn.Close()
}

func (cw *connectionWrapper) ensureResponse() {
	cw.m.Lock()
	defer cw.m.Unlock()
	if cw.responseDone {
		return
	}
	defer func() { cw.responseDone = true }()

	resp, err := protocol.ReadResponse(context.TODO(), cw.Conn)
	if err != nil || resp.Err != nil {
		if err == nil {
			err = errors.New(*resp.Err)
		}
		log.Printf("Dial error: %v", err)
		_ = cw.Conn.Close()
		_, _ = io.Copy(ioutil.Discard, cw.Conn)
		return
	}
}
