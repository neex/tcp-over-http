package tun

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/link/fdbased"
	"github.com/google/netstack/tcpip/link/rawfile"
	"github.com/google/netstack/tcpip/link/tun"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/network/ipv6"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	"github.com/google/netstack/waiter"
	log "github.com/sirupsen/logrus"

	"tcp-over-http/common"
)

func ForwardTCPFromTUN(tunName string, forward common.DialContextFunc) error {
	macAddr, err := net.ParseMAC("de:ad:be:ee:ee:ef")
	if err != nil {
		panic(err)
	}

	s := stack.New([]string{ipv4.ProtocolName, ipv6.ProtocolName, arp.ProtocolName},
		[]string{tcp.ProtocolName},
		stack.Options{})

	mtu, err := rawfile.GetMTU(tunName)
	if err != nil {
		return err
	}

	fd, err := tun.Open(tunName)
	if err != nil {
		return err
	}

	linkID, err := fdbased.New(&fdbased.Options{
		FDs:            []int{fd},
		MTU:            mtu,
		EthernetHeader: false,
		Address:        tcpip.LinkAddress(macAddr),
	})

	if err != nil {
		return err
	}

	if err := s.CreateNIC(1, linkID); err != nil {
		return errors.New(err.String())
	}

	if err := s.AddAddress(1, arp.ProtocolNumber, arp.ProtocolAddress); err != nil {
		return errors.New(err.String())
	}

	var routes []tcpip.Route
	for i, proto := range []tcpip.NetworkProtocolNumber{ipv4.ProtocolNumber, ipv6.ProtocolNumber} {
		zero := strings.Repeat("\x00", 4+12*i)
		addr := tcpip.Address(zero)
		mask := tcpip.AddressMask(zero)

		nw, err := tcpip.NewSubnet(addr, mask)
		if err != nil {
			return err
		}

		if err := s.AddSubnet(1, proto, nw); err != nil {
			return errors.New(err.String())
		}

		routes = append(routes, tcpip.Route{Destination: addr, Mask: mask, NIC: 1})
	}

	s.SetRouteTable(routes)

	fwd := tcp.NewForwarder(s, 0, 10, func(r *tcp.ForwarderRequest) {
		wq := new(waiter.Queue)
		ep, err := r.CreateEndpoint(wq)
		r.Complete(false)
		if err != nil {
			id := r.ID()
			addr := net.TCPAddr{IP: []byte(id.LocalAddress), Port: int(id.LocalPort)}
			log.WithField("addr", addr).WithError(errors.New(err.String())).Error("endpoint not created")
			return
		}
		forwardTCP(wq, ep, forward)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)
	return nil
}

func forwardTCP(wq *waiter.Queue, ep tcpip.Endpoint, forward common.DialContextFunc) {
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

	log.WithField("addr", tcpAddr.String()).Info("forward from tun")

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

func init() {
	rand.Seed(time.Now().UnixNano())
}
