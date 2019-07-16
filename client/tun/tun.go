package tun

import (
	"errors"
	"math/rand"
	"net"
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
	"github.com/google/netstack/tcpip/transport/udp"
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
		[]string{tcp.ProtocolName, udp.ProtocolName},
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

	s.SetPromiscuousMode(1, true)

	tcpForwarder := tcp.NewForwarder(s, 0, 10, func(r *tcp.ForwarderRequest) {
		wq := new(waiter.Queue)
		ep, err := r.CreateEndpoint(wq)
		if err != nil {
			id := r.ID()
			addr := net.TCPAddr{IP: []byte(id.LocalAddress), Port: int(id.LocalPort)}
			log.WithField("addr", addr).WithError(errors.New(err.String())).Error("tcp endpoint not created")
			return
		}
		r.Complete(false)
		ForwardTCPEndpoint(wq, ep, forward)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)

	udpForwarder := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		wq := new(waiter.Queue)
		ep, err := r.CreateEndpoint(wq)
		id := r.ID()
		addr := net.UDPAddr{IP: []byte(id.LocalAddress), Port: int(id.LocalPort)}
		if err != nil {
			log.WithField("addr", addr).WithError(errors.New(err.String())).Error("tcp endpoint not created")
			return
		}
		if addr.String() == "8.8.8.8:53" {
			// TODO: spoof dns
		}
		go ForwardUDPEndpoint(wq, ep, forward)
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)

	return nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
