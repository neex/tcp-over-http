package tun

import (
	"sync"
	"time"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/waiter"

	"github.com/neex/tcp-over-http/client/tun/dns"
)

func HandleDNS(wq *waiter.Queue, ep tcpip.Endpoint, forwarder *dns.Forwarder) {
	defer ep.Close()

	log := GetLogger(ep)

	waitEntry, notifyCh := waiter.NewChannelEntry(nil)
	wq.EventRegister(&waitEntry, waiter.EventIn)
	defer wq.EventUnregister(&waitEntry)

	var wg sync.WaitGroup

	for {
		v, _, netStackErr := ep.Read(nil)
		if netStackErr != nil {
			if netStackErr == tcpip.ErrWouldBlock {
				t := time.NewTimer(10 * time.Second)
				hasData := false
				select {
				case <-notifyCh:
					hasData = true
				case <-t.C:
				}
				t.Stop()

				if hasData {
					continue
				}
			}

			break
		}
		log.Info("dns request received")
		wg.Add(1)
		forwarder.SendRequest([]byte(v), func(_ []byte, reply []byte) {
			log.Info("dns reply received")
			defer wg.Done()
			if _, _, err := ep.Write(tcpip.SlicePayload(reply), tcpip.WriteOptions{}); err != nil {
				ep.Close()
			}
		})
	}

	wg.Wait()
	ep.Close()
}
