package main

import (
	"context"
	"net"
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"

	"tcp-over-http/common"
)

func DirectDialMiddleware(directHosts *regexp.Regexp,
	next common.DialContextFunc,
	timeout time.Duration) common.DialContextFunc {

	directDialer := net.Dialer{Timeout: timeout}

	return func(ctx context.Context, network, address string) (conn net.Conn, e error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}

		if directHosts.MatchString(host) {
			logger := log.WithField("remote", address)
			logger.Info("dialing without proxy")
			conn, e = directDialer.DialContext(ctx, network, address)
			if e != nil {
				logger.WithError(e).Error("error while directly dialing")
			}
			return
		}

		return next(ctx, network, address)
	}
}
