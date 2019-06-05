package client

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type Connector struct {
	Config *Config
}

func (c *Connector) Connect(logger *log.Entry) (*MultiplexedConnection, error) {
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
	logger.Info("connecting to upstream")
	conn, err := tls.DialWithDialer(d, "tcp", host, &tls.Config{
		NextProtos: []string{"http/1.1"},
	})

	if err != nil {
		if conn != nil {
			_ = conn.Close()
		}
		logger.WithError(err).Error("while connecting to upstream")
		return nil, err
	}

	req, err := http.NewRequest("GET", c.Config.Address, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		logger.WithError(err).Error("error while writing initial http request")
		return nil, err
	}

	logger.Debug("lazy upstream connect successful")

	cw := &connectionWrapper{
		Conn:   conn,
		logger: logger,
	}

	connCfg := &MultiplexedConnectionConfig{
		MaxMultiplexedConnections: c.Config.MaxConnectionMultiplex,
		RemoteDialTimeout:         c.Config.ConnectTimeout,
		KeepAliveTimeout:          c.Config.KeepAliveTimeout,
		Logger:                    logger,
	}

	return NewMultiplexedConnection(cw, connCfg)
}
