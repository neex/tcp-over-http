package client

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"net/url"
)

type Connector struct {
	Config *Config
}

func (c *Connector) Connect() (*MultiplexedConnection, error) {
	parsed, err := url.Parse(c.Config.Address)
	if err != nil {
		return nil, err
	}

	host := parsed.Host

	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, parsed.Scheme)
	}

	d := &net.Dialer{
		Timeout: c.Config.ConnectTimeout,
	}
	conn, err := tls.DialWithDialer(d, "tcp", host, &tls.Config{
		NextProtos: []string{"http/1.1"},
	})

	if err != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, err
	}

	req, err := http.NewRequest("GET", c.Config.Address, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	cw := &connectionWrapper{
		onDisconnect: func() {
			log.Printf("Upstream connection disconnected")
		},
		Conn: conn,
	}
	return NewMultiplexedConnection(cw, c.Config.MaxConnectionMultiplex, c.Config.RemoteTimeout)
}
