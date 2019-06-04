package main

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"tcp-over-http/client"
	socks5_server "tcp-over-http/client/socks5-server"
)

func main() {
	var dialer *client.Dialer

	cmdDial := &cobra.Command{
		Use:   "dial [addr to dial]",
		Short: "Dial to addr and connect to stdin/stdout",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dialer.Verbose = false

			addr := args[0]

			conn, err := dialer.DialContext(context.Background(), "tcp", addr)
			if err != nil {
				log.Fatal(err)
			}

			forward(conn, os.Stdin, os.Stdout)
		},
	}

	cmdForward := &cobra.Command{
		Use:   "forward [local addr] [remote addr]",
		Short: "Listen on local addr and forward every connection to remote addr",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			localAddr := args[0]
			remoteAddr := args[1]

			lsn, err := net.Listen("tcp", localAddr)
			if err != nil {
				log.Fatal(err)
			}

			for {
				c, err := lsn.Accept()
				if err != nil {
					log.Fatal(err)
				}

				go func(c net.Conn) {
					conn, err := dialer.DialContext(context.Background(), "tcp", remoteAddr)
					if err != nil {
						log.Print(err)
					}

					forward(conn, c, c)
				}(c)
			}
		},
	}

	cmdSocks5 := &cobra.Command{
		Use:   "socks5 [local addr]",
		Short: "Run socks5 server on local addr",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			localAddr := args[0]
			server := &socks5_server.Socks5Server{
				Dialer: dialer.DialContext,
			}

			if err := server.ListenAndServe(context.Background(), localAddr); err != nil {
				log.Fatal(err)
			}
		},
	}

	var configFilename string
	rootCmd := &cobra.Command{Use: "tcp_over_http"}
	rootCmd.AddCommand(cmdDial, cmdForward, cmdSocks5)
	rootCmd.PersistentFlags().StringVarP(&configFilename, "config", "c", "./config.yaml", "path to config")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		config, err := client.NewConfigFromFile(configFilename)
		if err != nil {
			return err
		}
		connector := &client.Connector{Config: config}
		dialer = &client.Dialer{Connector: connector, Verbose: true}
		return nil
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func forward(conn net.Conn, in io.Reader, out io.WriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, in)
		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(out, conn)
		_ = out.Close()
	}()

	wg.Wait()
}
