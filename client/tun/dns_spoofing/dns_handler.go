package dns_spoofing

import (
	"sync"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/waiter"
)

type Spoofer struct {
	m    sync.Mutex
	addr map[string]string
}

func (s *Spoofer) HandleDNS(wq *waiter.Queue, ep tcpip.Endpoint) {
	defer ep.Close()

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

		_ = v
		panic("not implemented")
	}
}
