package tun

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/google/netstack/tcpip/adapters/gonet"

	"github.com/neex/tcp-over-http/client/forwarder"

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
)

func ForwardTransportFromTUN(tunName string, f *forwarder.Forwarder) error {
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
		forwardNetstackEndpoint(wq, ep, "tcp", f)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)

	udpForwarder := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		wq := new(waiter.Queue)
		ep, err := CreateUDPEndpoint(wq, r)
		id := r.ID()
		addr := net.UDPAddr{IP: []byte(id.LocalAddress), Port: int(id.LocalPort)}
		if err != nil {
			log.WithField("addr", addr).WithError(errors.New(err.String())).Error("udp endpoint not created")
			return
		}
		go forwardNetstackEndpoint(wq, ep, "udp", f)
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)

	return nil
}

func forwardNetstackEndpoint(wq *waiter.Queue, ep tcpip.Endpoint, network string, f *forwarder.Forwarder) {
	defer ep.Close()
	logger := GetLogger(ep).WithField("network", network)

	conn := gonet.NewConn(wq, ep)
	logger.Info("forward from tun")
	addr, netstackErr := ep.GetLocalAddress()
	if netstackErr != nil {
		logger.WithError(errors.New(netstackErr.String())).Error("error while retrieving localAddr")
		return
	}
	addrString := net.JoinHostPort(net.IP([]byte(addr.Addr)).String(), strconv.Itoa(int(addr.Port)))

	err := f.ForwardConnection(context.TODO(), &forwarder.ForwardRequest{
		ClientConn: conn,
		Network:    network,
		Address:    addrString,
	})
	if err != nil {
		logger.WithError(err).Error("forwarder returned error")
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
