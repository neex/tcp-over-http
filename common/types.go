package common

import (
	"context"
	"net"
)

type DialContextFunc = func(ctx context.Context, network, address string) (net.Conn, error)
