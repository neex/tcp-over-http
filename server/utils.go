package server

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

func CheckHost(config *Config, handler http.Handler) http.Handler {
	if config.Domain == "" {
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != config.Domain {
			log.WithFields(log.Fields{
				"host":        r.Host,
				"remote_addr": r.RemoteAddr,
			}).Warn("request with wrong host")
			w.WriteHeader(404)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
