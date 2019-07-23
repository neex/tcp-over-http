package tun

import (
	"context"
	"sync"
	"tcp-over-http/protocol"
	"time"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/waiter"

	"tcp-over-http/common"
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

	var m sync.Mutex
	lastAction := time.Now()
	actionHappened := func() {
		m.Lock()
		defer m.Unlock()
		lastAction = time.Now()
	}

	getTimeout := func() time.Duration {
		m.Lock()
		defer m.Unlock()
		return time.Now().Add(10 * time.Second).Sub(lastAction)
	}

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
					timeLeft := getTimeout()
					if timeLeft < 0 {
						break
					}

					timeoutTimer := time.NewTimer(timeLeft)
					select {
					case <-timeoutTimer.C:
					case <-notifyCh:
					}
					timeoutTimer.Stop()
					continue
				}

				break
			}
			actionHappened()
			_, err := conn.Write(v)
			if err != nil {
				log.WithError(err).Error("dns response not written")
				break
			}
		}

		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()
		for {
			buf := make([]byte, 65536)
			if err := conn.SetDeadline(time.Now().Add(getTimeout())); err != nil {
				log.WithError(err).Error("error while setting deadline")
				break
			}
			size, err := conn.Read(buf)
			if err != nil {
				break
			}
			actionHappened()
			toWrite := buf[:size]
			_, _, netStackErr := ep.Write(tcpip.SlicePayload(toWrite), tcpip.WriteOptions{})
			if netStackErr != nil {
				break
			}
		}
		ep.Close()
	}()

	wg.Wait()
}
