package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	log "github.com/sirupsen/logrus"

	"tcp-over-http/common"
	"tcp-over-http/protocol"
)

func RunMultiplexedServer(ctx context.Context, conn net.Conn, dial common.DialContextFunc) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-newCtx.Done()
		_ = conn.Close()
	}()

	packet := &protocol.ConnectionResponse{
		Err:     nil,
		Padding: hex.EncodeToString(make([]byte, rand.Intn(2)*500+500)),
	}

	if err := protocol.WritePacket(ctx, conn, packet); err != nil {
		return fmt.Errorf("error while writing initial response: %v", err)
	}

	conf := *yamux.DefaultConfig()
	conf.LogOutput = log.StandardLogger().WriterLevel(log.ErrorLevel)
	sess, err := yamux.Server(conn, &conf)
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

var allowedNets = map[string]struct{}{
	"tcp":  {},
	"udp":  {},
	"tcp6": {},
	"udp6": {},
}

func processClient(ctx context.Context, conn net.Conn, dial common.DialContextFunc) error {
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
	if _, ok := allowedNets[req.Network]; !ok {
		err := fmt.Sprintf("Network %#v not allowed", req.Network)
		return protocol.WritePacket(newCtx, conn, &protocol.ConnectionResponse{Err: &err})
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
