package netstack_hacks

import (
	"reflect"
	"unsafe"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/adapters/gonet"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/udp"
	"github.com/google/netstack/waiter"
)

func CreateUDPEndpoint(wq *waiter.Queue, r *udp.ForwarderRequest) (tcpip.Endpoint, *tcpip.Error) {
	rVal := reflect.Indirect(reflect.ValueOf(r))
	routePtr := (**stack.Route)(unsafe.Pointer(rVal.FieldByName("route").UnsafeAddr()))
	proto := (*routePtr).NetProto

	ep, err := r.CreateEndpoint(wq)
	if err != nil {
		return ep, err
	}

	epVal := reflect.Indirect(reflect.ValueOf(ep))
	type protos = []tcpip.NetworkProtocolNumber
	protosPtr := (*protos)(unsafe.Pointer(epVal.FieldByName("effectiveNetProtos").UnsafeAddr()))
	*protosPtr = protos{proto}
	return ep, err
}

func CreatePacketConn(s *stack.Stack, ep tcpip.Endpoint, wq *waiter.Queue) *gonet.PacketConn {
	c := &gonet.PacketConn{}
	cVal := reflect.Indirect(reflect.ValueOf(c))

	stackPtr := (**stack.Stack)(unsafe.Pointer(cVal.FieldByName("stack").UnsafeAddr()))
	*stackPtr = s

	epPtr := (*tcpip.Endpoint)(unsafe.Pointer(cVal.FieldByName("ep").UnsafeAddr()))
	*epPtr = ep

	wqPtr := (**waiter.Queue)(unsafe.Pointer(cVal.FieldByName("wq").UnsafeAddr()))
	*wqPtr = wq

	timerVal := cVal.FieldByName("deadlineTimer")
	readCancelChPtr := (*chan struct{})(unsafe.Pointer(timerVal.FieldByName("readCancelCh").UnsafeAddr()))
	*readCancelChPtr = make(chan struct{})

	writeCancelChPtr := (*chan struct{})(unsafe.Pointer(timerVal.FieldByName("writeCancelCh").UnsafeAddr()))
	*writeCancelChPtr = make(chan struct{})
	return c
}
