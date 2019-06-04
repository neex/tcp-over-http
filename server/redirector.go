package server

import "net/http"

func RunRedirectorServer(config *Config) error {
	return http.ListenAndServe(config.RedirectorAddr, CheckHost(config, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			url := *r.URL
			url.Scheme = "https"
			url.Host = r.Host
			http.Redirect(w, r, url.String(), 301)
		},
	)))
}
