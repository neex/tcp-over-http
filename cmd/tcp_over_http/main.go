package main

import (
	"context"
	"io"
	"net"
	"os"
	"regexp"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/neex/tcp-over-http/client"
	socks5server "github.com/neex/tcp-over-http/client/socks5-server"
	"github.com/neex/tcp-over-http/client/tun"
)

func main() {
	var (
		dialer             *client.Dialer
		logLevel           string
		remoteNet          string
		directDialRegexp   string
		directDialCompiled *regexp.Regexp
		poolSize           int
		tunDevice          string
	)

	cmdDial := &cobra.Command{
		Use:   "dial [addr to dial]",
		Short: "Dial to addr and connect to stdin/stdout",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if logLevel == "" {
				log.SetLevel(log.ErrorLevel)
			}

			addr := args[0]

			conn, err := dialer.DialContext(context.Background(), remoteNet, addr)
			if err != nil {
				log.WithError(err).Fatal("dial failed")
			}

			forward(conn, os.Stdin, os.Stdout)
		},
	}
	cmdDial.PersistentFlags().StringVar(&remoteNet, "remote-net", "tcp", "remote network (tcp/udp)")

	cmdForward := &cobra.Command{
		Use:   "forward [local addr] [remote addr]",
		Short: "Listen on local addr and forward every connection to remote addr",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			localAddr := args[0]
			remoteAddr := args[1]

			dialer.PreconnectPoolSize = poolSize
			dialer.EnablePreconnect()

			lsn, err := net.Listen("tcp", localAddr)
			if err != nil {
				log.WithError(err).Fatal("listen failed")
			}

			log.Info("forward server started")

			for {
				c, err := lsn.Accept()
				if err != nil {
					log.WithError(err).Fatal("accept failed")
				}

				go func(c net.Conn) {
					conn, err := dialer.DialContext(context.Background(), remoteNet, remoteAddr)
					if err != nil {
						log.WithError(err).Error("dial failed early")
						return
					}

					forward(conn, c, c)
				}(c)
			}
		},
	}
	cmdForward.PersistentFlags().IntVar(&poolSize, "preconnect-pool", 5, "preconnect pool size")
	cmdForward.PersistentFlags().StringVar(&remoteNet, "remote-net", "tcp", "remote network (tcp/udp)")

	cmdProxy := &cobra.Command{
		Use:   "proxy [local addr]",
		Short: "Run socks5 server on local addr, optionally also forward tun connections",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			localAddr := args[0]

			if poolSize > 0 {
				dialer.PreconnectPoolSize = poolSize
				dialer.EnablePreconnect()
				go func() {
					for range time.Tick(10 * time.Second) {
						t, err := dialer.Ping()
						if err != nil {
							log.WithError(err).Error("ping error")
							continue
						}
						log.WithField("roundtrip", t).Info("ping")
					}
				}()
			}

			if tunDevice != "" {
				if err := tun.ForwardTransportFromTUN(tunDevice, dialer.DialContext); err != nil {
					log.WithError(err).Fatal("tun forward failed")
				}
			}

			server := &socks5server.Socks5Server{
				Dialer: dialer.DialContext,
			}

			if directDialCompiled != nil {
				server.Dialer = DirectDialMiddleware(directDialCompiled, 20*time.Second, server.Dialer)
			}

			if err := server.ListenAndServe(context.Background(), localAddr); err != nil {
				log.WithError(err).Fatal("socks5 listen failed")
			}
		},
	}
	cmdProxy.PersistentFlags().IntVar(&poolSize, "preconnect-pool", 5, "preconnect pool size")
	cmdProxy.PersistentFlags().StringVar(&directDialRegexp, "direct-dial", "", "the regexp for addresses that should be dialed without proxy")
	cmdProxy.PersistentFlags().StringVar(&tunDevice, "tun", "", "tun device to listen on")
	cmdProxy.PreRunE = func(cmd *cobra.Command, args []string) error {
		if directDialRegexp == "" {
			return nil
		}
		var err error
		directDialCompiled, err = regexp.Compile(directDialRegexp)
		if err != nil {
			return err
		}
		return nil
	}

	var configFilename string
	rootCmd := &cobra.Command{Use: "tcp_over_http"}
	rootCmd.AddCommand(cmdDial, cmdForward, cmdProxy)
	rootCmd.PersistentFlags().StringVarP(&configFilename, "config", "c", "./config.yaml", "path to config")
	rootCmd.PersistentFlags().StringVar(&logLevel, "loglevel", "", "loglevel")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if logLevel != "" {
			if logLevel == "trace_netop" {
				client.TraceNetOps = true
				logLevel = "trace"
			}
			level, err := log.ParseLevel(logLevel)
			if err != nil {
				return err
			}

			log.SetLevel(level)
		}
		config, err := client.NewConfigFromFile(configFilename)
		if err != nil {
			return err
		}
		connector := &client.Connector{Config: config}
		dialer = &client.Dialer{Connector: connector}
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
