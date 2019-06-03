package server

import (
	"log"
	"net/http"
)

func CheckHost(config *Config, handler http.Handler) http.Handler {
	if config.Domain == "" {
		return handler
	}

	return http.HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != config.Domain {
			log.Printf("request with wrong host: %v", r.Host)
			w.WriteHeader(404)
			return
		}
		handler.ServeHTTP(w, r)
	}))
}
