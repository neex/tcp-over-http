package client

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/neex/tcp-over-http/protocol"
)

type connectionWrapper struct {
	responseOnce   sync.Once
	disconnectOnce sync.Once
	onDisconnect   func()
	logger         *log.Entry

	net.Conn
}

func (cw *connectionWrapper) Read(b []byte) (n int, err error) {
	cw.responseOnce.Do(cw.ensureResponse)
	return cw.Conn.Read(b)
}

func (cw *connectionWrapper) Close() (err error) {
	err = cw.Conn.Close()
	cw.disconnectOnce.Do(func() {
		cw.logger.Info("disconnected")

		if cw.onDisconnect != nil {
			cw.onDisconnect()
		}

	})
	return
}

func (cw *connectionWrapper) ensureResponse() {
	cw.logger.Trace("reading initial response")
	resp, err := protocol.ReadResponse(context.TODO(), cw.Conn)
	if err != nil || resp.Err != nil {
		if err != nil {
			cw.logger.WithError(err).Warn("error while dialing")
		} else {
			cw.logger.WithError(err).Error("error while dialing")
		}

		_ = cw.Conn.Close()
		_, _ = io.Copy(ioutil.Discard, cw.Conn)
		return
	}

	cw.logger.Debug("remote end connected")
}
