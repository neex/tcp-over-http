package client

import "time"

type Config struct {
	Address                string
	RemoteTimeout          time.Duration
	ConnectTimeout         time.Duration
	MaxConnectionMultiplex int
}
