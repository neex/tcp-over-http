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

const udpReceiveTimeout = 15 * time.Second

func ForwardUDPEndpoint(wq *waiter.Queue, ep tcpip.Endpoint, forward common.DialContextFunc) {
	defer ep.Close()
	netstackAddr, netstackErr := ep.GetLocalAddress()
	if netstackErr != nil {
		log.WithError(errors.New(netstackErr.String())).Error("error while retrieving localAddr")
		return
	}
	udpAddr := net.UDPAddr{IP: net.IP([]byte(netstackAddr.Addr)), Port: int(netstackAddr.Port)}

	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	conn, err := forward(ctx, udpAddr.Network(), udpAddr.String())
	cancel()
	if err != nil {
		log.WithField("addr", udpAddr).WithError(err).Error("error while dialing")
		return
	}

	log.WithField("addr", udpAddr.String()).Info("forward from tun (udp)")

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
		return time.Now().Add(udpReceiveTimeout).Sub(lastAction)
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
				log.WithField("addr", udpAddr.String()).WithError(err).Error("error while setting deadline")
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
