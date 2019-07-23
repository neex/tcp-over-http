package tun

import (
	"net"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/udp"
	"github.com/google/netstack/waiter"

	"github.com/google/netstack/tcpip"
	log "github.com/sirupsen/logrus"
)

func GetLogger(ep tcpip.Endpoint) *log.Entry {
	localAddr := "<error>"
	if addr, err := ep.GetLocalAddress(); err == nil {
		localAddr = net.JoinHostPort(net.IP(addr.Addr).String(), strconv.Itoa(int(addr.Port)))
	}
	remoteAddr := "<error>"
	if addr, err := ep.GetRemoteAddress(); err == nil {
		remoteAddr = net.JoinHostPort(net.IP(addr.Addr).String(), strconv.Itoa(int(addr.Port)))
	}
	return log.WithField("remote", localAddr).WithField("local", remoteAddr)
}

func CreateUDPEndpoint(wq *waiter.Queue, r *udp.ForwarderRequest) (tcpip.Endpoint, *tcpip.Error) {
	rVal := reflect.Indirect(reflect.ValueOf(r))
	routeVal := rVal.FieldByName("route")
	routePtr := unsafe.Pointer(routeVal.UnsafeAddr())
	proto := (*(**stack.Route)(routePtr)).NetProto

	ep, err := r.CreateEndpoint(wq)
	if err != nil {
		return ep, err
	}

	epVal := reflect.Indirect(reflect.ValueOf(ep))
	protosVal := epVal.FieldByName("effectiveNetProtos")
	protosPtr := unsafe.Pointer(protosVal.UnsafeAddr())
	protos := (*[]tcpip.NetworkProtocolNumber)(protosPtr)
	*protos = []tcpip.NetworkProtocolNumber{proto}
	return ep, err
}
