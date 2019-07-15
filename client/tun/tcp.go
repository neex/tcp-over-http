package tun

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/waiter"
	log "github.com/sirupsen/logrus"

	"tcp-over-http/common"
)

func ForwardTCPEndpoint(wq *waiter.Queue, ep tcpip.Endpoint, forward common.DialContextFunc) {
	defer ep.Close()
	netstackAddr, netstackErr := ep.GetLocalAddress()
	if netstackErr != nil {
		log.WithError(errors.New(netstackErr.String())).Error("error while retrieving localAddr")
		return
	}
	tcpAddr := net.TCPAddr{IP: net.IP([]byte(netstackAddr.Addr)), Port: int(netstackAddr.Port)}

	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	conn, err := forward(ctx, tcpAddr.Network(), tcpAddr.String())
	cancel()
	if err != nil {
		log.WithField("addr", tcpAddr).WithError(err).Error("error while dialing")
		return
	}

	log.WithField("addr", tcpAddr.String()).Info("forward from tun (tcp)")

	defer func() { _ = conn.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		waitEntry, notifyCh := waiter.NewChannelEntry(nil)
		wq.EventRegister(&waitEntry, waiter.EventIn)
		defer wq.EventUnregister(&waitEntry)

		for {
			v, _, netStackErr := ep.Read(nil)
			if netStackErr != nil {
				if netStackErr == tcpip.ErrWouldBlock {
					<-notifyCh
					continue
				}

				break
			}

			_, err := conn.Write(v)
			if err != nil {
				break
			}
		}

		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()

		waitEntry, notifyCh := waiter.NewChannelEntry(nil)
		wq.EventRegister(&waitEntry, waiter.EventOut)
		defer wq.EventUnregister(&waitEntry)

	writeLoop:
		for {
			buf := make([]byte, 32*1024)
			size, err := conn.Read(buf)
			if err != nil {
				break
			}

			var totalWritten uint64
			for totalWritten < uint64(size) {
				toWrite := buf[totalWritten:size]
				written, _, netStackErr := ep.Write(tcpip.SlicePayload(toWrite), tcpip.WriteOptions{})
				if written > 0 {
					totalWritten += uint64(written)
					continue
				}

				if netStackErr == tcpip.ErrWouldBlock {
					<-notifyCh
					continue
				}

				break writeLoop
			}
		}

		ep.Close()
	}()

	wg.Wait()
}
