package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
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
	mux.Handle("/", static)

	mux.HandleFunc(fmt.Sprintf("/establish/%s", config.Token), func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			log.Print("Connection doesn't support http.Hijacker")
			w.WriteHeader(500)
			return
		}

		conn, br, err := hj.Hijack()
		if err != nil {
			log.Print("Error while hijacking connection:", err)
			w.WriteHeader(500)
			return
		}

		d := net.Dialer{
			Timeout: config.DialTimeout,
		}

		hc := &hijackedConn{br: br, Conn: conn}
		if err := RunMultiplexedServer(r.Context(), hc, d.DialContext); err != nil {
			log.Printf("Connection handling ended with error: %v", err)
		}
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
