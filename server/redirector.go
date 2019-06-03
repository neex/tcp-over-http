package server

import (
	"net/http"
)

func RunRedirectorServer(config *Config) error {
	return http.ListenAndServe(config.RedirectorAddr, CheckHost(config, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			url := *r.URL
			url.Scheme = "https"
			http.Redirect(w, r, url.String(), 301)
		},
	)))
}
