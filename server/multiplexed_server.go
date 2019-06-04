package server

import (
	"context"
	"fmt"
	"io"
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

	if err := protocol.WritePacket(ctx, conn, &protocol.ConnectionResponse{Err: nil}); err != nil {
		return fmt.Errorf("error while writing initial response: %v", err)
	}

	sess, err := yamux.Server(conn, nil)
	if err != nil {
		return fmt.Errorf("error while creating server: %v", err)
	}

	for {
		client, err := sess.Accept()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("error while accept: %v", err)
		}

		go func() {
			_ = processClient(newCtx, client, dial)
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

	var errStr *string
	if err != nil {
		errStr = new(string)
		*errStr = err.Error()
	}

	writeErr := protocol.WritePacket(newCtx, conn, &protocol.ConnectionResponse{Err: errStr})
	if err != nil {
		return err
	}
	if writeErr != nil {
		return writeErr
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, upstreamConn)
		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstreamConn, conn)
		_ = upstreamConn.Close()
	}()

	wg.Wait()
	return nil
}
