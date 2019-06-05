package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"
)

func RunHTTPServer(config *Config) error {
	mux := makeHTTPMux(config)
	srv := http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	if config.IsHTTPS() {
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{config.Certificate},
		}

		return srv.ListenAndServeTLS("", "")
	}

	return srv.ListenAndServe()
}

func makeHTTPMux(config *Config) http.Handler {
	mux := http.NewServeMux()
	static := http.FileServer(http.Dir(config.StaticDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.WithFields(log.Fields{
			"remote_addr": r.RemoteAddr,
			"remote_uri":  r.RequestURI,
		}).Info("static request")
		static.ServeHTTP(w, r)
	})

	mux.HandleFunc(fmt.Sprintf("/establish/%s", config.Token), func(w http.ResponseWriter, r *http.Request) {
		l := log.WithFields(log.Fields{
			"remote_addr": r.RemoteAddr,
		})

		l.Info("proxy request")
		hj, ok := w.(http.Hijacker)
		if !ok {
			l.Error("connection doesn't support http.Hijacker")
			w.WriteHeader(500)
			return
		}

		conn, br, err := hj.Hijack()
		if err != nil {
			l.WithError(err).Error("error while hijacking connection")
			w.WriteHeader(500)
			return
		}

		d := net.Dialer{
			Timeout: config.DialTimeout,
		}

		hc := &hijackedConn{br: br, Conn: conn}
		if err := RunMultiplexedServer(r.Context(), hc, d.DialContext); err != nil {
			l.WithError(err).Error("connection handling ended with error")
		}

		l.Info("proxy request finished")
	})

	return CheckHost(config, mux)
}

type hijackedConn struct {
	br *bufio.ReadWriter
	net.Conn
}

func (hc *hijackedConn) Read(b []byte) (int, error) {
	return hc.br.Read(b)
}
