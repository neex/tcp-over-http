package server

import (
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
		if config.Domain != "" {
			srv.TLSConfig = &tls.Config{
				GetCertificate: func(info *tls.ClientHelloInfo) (cert *tls.Certificate, err error) {
					if info.ServerName == config.Domain {
						return &config.Certificate, nil
					}
					return nil, nil
				},
			}
		} else {
			srv.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{config.Certificate},
			}
		}

		return srv.ListenAndServeTLS("", "")
	}

	return srv.ListenAndServe()
}

func makeHTTPMux(config *Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, config.StaticDir)
	})

	mux.HandleFunc(fmt.Sprintf("/establish/%s", config.Token), func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			log.Print("Connection doesn't support http.Hijacker")
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)

		conn, _, err := hj.Hijack()
		if err != nil {
			log.Print("Error while hijacking connection:", err)
			w.WriteHeader(500)
			return
		}

		d := net.Dialer{
			Timeout: config.DialTimeout,
		}
		if err := RunMultiplexedServer(r.Context(), conn, d.DialContext); err != nil {
			log.Printf("Connection handling ended with error: %v", err)
		}
	})

	return CheckHost(config, mux)
}
