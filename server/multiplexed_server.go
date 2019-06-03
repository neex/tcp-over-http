package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/hashicorp/yamux"

	"tcp-over-http/protocol"
)

type DialContextFunc = func(ctx context.Context, network, address string) (net.Conn, error)

func RunMultiplexedServer(ctx context.Context, conn net.Conn, dial DialContextFunc) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-newCtx.Done()
		_ = conn.Close()
	}()

	sess, err := yamux.Server(conn, nil)
	if err != nil {
		return fmt.Errorf("error while creating server: %v", err)
	}

	for {
		client, err := sess.Accept()
		if err != nil {
			return fmt.Errorf("error while accept: %v", err)
		}
		go func() {
			if err := processClient(newCtx, client, dial); err != nil {
				log.Printf("Error handling client: %v", err)
			}
		}()
	}
}

func processClient(ctx context.Context, conn net.Conn, dial DialContextFunc) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-newCtx.Done()
		_ = conn.Close()
	}()

	req, err := protocol.ReadRequest(newCtx, conn)
	if err != nil {
		return err
	}

	dialCtx, cancelDialCtx := context.WithTimeout(newCtx, req.Timeout)
	upstreamConn, err := dial(dialCtx, req.Network, req.Address)
	if upstreamConn != nil {
		defer func() { _ = upstreamConn.Close() }()
	}
	cancelDialCtx()

	writeErr := protocol.WritePacket(newCtx, conn, protocol.ConnectionResponse{Err: err})
	if err != nil {
		log.Printf("Connection to %v://%v failed: %v", req.Network, req.Address, err)
		return err
	}
	log.Printf("Connection to %v://%v established", req.Network, req.Address)

	if writeErr != nil {
		return writeErr
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		_, _ = io.Copy(conn, upstreamConn)
		_ = conn.Close()
	}()

	go func() {
		_, _ = io.Copy(upstreamConn, conn)
		_ = upstreamConn.Close()
	}()

	wg.Wait()
	return nil
}
