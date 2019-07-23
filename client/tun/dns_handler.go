package tun

import (
	"context"
	"sync"
	"time"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/waiter"

	"tcp-over-http/common"
	"tcp-over-http/protocol"
)

func HandleDNS(wq *waiter.Queue, ep tcpip.Endpoint, forward common.DialContextFunc) {
	defer ep.Close()

	log := GetLogger(ep)

	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	connRaw, err := forward(ctx, "tcp", "1.1.1.1:53")
	cancel()
	if err != nil {
		log.WithError(err).Error("error while dialing")
		return
	}
	conn := protocol.NewPacketConnection(connRaw)
	log.Info("forward from tun (dns)")

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
				log.WithError(err).Error("dns request not written")
				break
			}
		}

		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()
		for {
			buf := make([]byte, 65536)
			size, err := conn.Read(buf)
			if err != nil {
				break
			}
			toWrite := buf[:size]
			if _, _, err := ep.Write(tcpip.SlicePayload(toWrite), tcpip.WriteOptions{}); err != nil {
				break
			}
		}
		ep.Close()
	}()

	wg.Wait()
}
