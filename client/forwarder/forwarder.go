package forwarder

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/neex/tcp-over-http/common"
)

type Forwarder struct {
	Dial        common.DialContextFunc
	DialTimeout time.Duration
}

type ForwardRequest struct {
	ClientConn       net.Conn
	Network, Address string
	OnConnected      func()
}

func (f *Forwarder) ForwardConnection(ctx context.Context, r *ForwardRequest) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-newCtx.Done()
		_ = r.ClientConn.Close()
	}()

	dialCtx, dialCtxCancel := context.WithTimeout(newCtx, f.DialTimeout)
	upstream, err := f.Dial(dialCtx, r.Network, r.Address)
	dialCtxCancel()
	if err != nil {
		return err
	}

	go func() {
		<-newCtx.Done()
		_ = upstream.Close()
	}()

	if r.OnConnected != nil {
		r.OnConnected()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel()
		packetCopy(upstream, r.ClientConn, 65536)
	}()

	go func() {
		defer wg.Done()
		defer cancel()
		packetCopy(r.ClientConn, upstream, 65536)
	}()

	wg.Wait()
	return nil
}

// packetCopy copies streams without quirks used by io.Copy. That is
// useful for connection where packet borders are important (e.g. udp).
func packetCopy(dst io.WriteCloser, src io.Reader, bufsize int) {
	defer func() { _ = dst.Close() }()
	buf := make([]byte, bufsize)
	for {
		n, _ := src.Read(buf)
		if n == 0 {
			break
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			break
		}
		if n == len(buf) {
			buf = make([]byte, len(buf)*2)
		}
	}
}
