// +build !linux

package tun

import (
	"errors"

	"github.com/neex/tcp-over-http/client/forwarder"
)

func ForwardTransportFromTUN(tunName string, f *forwarder.Forwarder) error {
	return errors.New("not implemented")
}
