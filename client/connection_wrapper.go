package client

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"tcp-over-http/protocol"
)

type connectionWrapper struct {
	m            sync.Mutex
	responseDone bool
	disconnected uint32
	onDisconnect func()
	logger       *log.Logger

	net.Conn
}

func (cw *connectionWrapper) Read(b []byte) (n int, err error) {
	cw.ensureResponse()
	return cw.Conn.Read(b)
}

func (cw *connectionWrapper) Close() error {
	if atomic.SwapUint32(&cw.disconnected, 1) == 0 {
		cw.logger.Print("disconnected")

		if cw.onDisconnect != nil {
			defer cw.onDisconnect()
		}
	}

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
			err = fmt.Errorf("remote reported: %v", err)
		} else {
			err = fmt.Errorf("local reading response: %v", err)
		}

		if atomic.LoadUint32(&cw.disconnected) == 0 {
			cw.logger.Printf("dial error: %v", err)
		} else {
			cw.logger.Printf("close called while reading initial response")
		}

		_ = cw.Conn.Close()
		_, _ = io.Copy(ioutil.Discard, cw.Conn)
		return
	}
}
