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

	"github.com/neex/tcp-over-http/common"
	"github.com/neex/tcp-over-http/protocol"
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

var isPacket = map[string]bool{
	"tcp":  false,
	"tcp4": false,
	"tcp6": false,
	"udp":  true,
	"udp4": true,
	"udp6": true,
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
	needPacket, ok := isPacket[req.Network]
	if !ok {
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

	if needPacket {
		conn = protocol.NewPacketConnection(conn)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 65536)
		_, _ = io.CopyBuffer(conn, upstreamConn, buf)
		_ = conn.Close()
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 65536)
		_, _ = io.CopyBuffer(upstreamConn, conn, buf)
		_ = upstreamConn.Close()
	}()

	wg.Wait()
	return nil
}
