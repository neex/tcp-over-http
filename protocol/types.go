package protocol

import (
	"time"
)

type ConnectionRequest struct {
	Network string
	Address string
	Timeout time.Duration
}

type ConnectionResponse struct {
	Err error
}
