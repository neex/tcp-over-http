package main

import (
	"log"
	"os"
	"tcp-over-http/server"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s <config>", os.Args[0])
	}

	config, err := server.NewConfigFromFile(os.Args[1])
	if err != nil {
		log.Fatalf("Loading config: %v", err)
	}

	if config.RedirectorAddr != "" {
		go func() {
			if err := server.RunRedirectorServer(config); err != nil {
				log.Fatalf("Running redirector: %v", err)
			}
		}()
	}

	if err := server.RunHTTPServer(config); err != nil {
		log.Fatalf("Running server: %v", err)
	}
}
