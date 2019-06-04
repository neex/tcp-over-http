package socks5_server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

type Socks5Server struct {
	Dialer func(ctx context.Context, network, address string) (net.Conn, error)
}

func (p *Socks5Server) ListenAndServe(ctx context.Context, addr string) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	lc := &net.ListenConfig{}
	lsn, err := lc.Listen(newCtx, "tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		<-newCtx.Done()
		_ = lsn.Close()
	}()

	for {
		conn, err := lsn.Accept()
		if err != nil {
			return err
		}

		go func() {
			err := p.handleConn(newCtx, conn)
			if err != nil {
				log.Printf("Conn handling ended with error: %v", err)
			}
		}()
	}
}

func (p *Socks5Server) handleConn(ctx context.Context, conn net.Conn) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-newCtx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, 1024)
	if n, err := io.ReadFull(conn, buf[:2]); n != 2 || err != nil {
		return fmt.Errorf("read short during first msg, %v", err)
	}

	if buf[0] != 5 {
		return fmt.Errorf("wrong first byte, %v", buf[0])
	}

	cntAuth := int(buf[1])
	if n, err := io.ReadFull(conn, buf[:cntAuth]); n != cntAuth || err != nil {
		return fmt.Errorf("read short during reading auth methods, %v", err)
	}

	var auth byte = 0xff

	for i := 0; i < cntAuth; i++ {
		if buf[i] == 0 {
			auth = 0
		}
	}

	buf[0] = 0x5
	buf[1] = auth

	if n, err := conn.Write(buf[:2]); n != 2 || err != nil {
		return fmt.Errorf("invalid connection attempt: write short during hello, %v", err)
	}

	if auth != 0 {
		return errors.New("auth not found")
	}

	if n, err := io.ReadFull(conn, buf[:4]); n != 4 || err != nil {
		return fmt.Errorf("read short during request, %v", err)
	}

	resp := make([]byte, 10)
	resp[0] = 5
	resp[3] = 1

	if buf[0] != 5 || buf[1] != 1 || buf[2] != 0 {
		resp[1] = 7
		_, _ = conn.Write(resp)
		return fmt.Errorf("invalid request, %v", buf[:4])
	}

	var host string
	switch buf[3] {
	case 1:
		if n, err := io.ReadFull(conn, buf[:4]); n != 4 || err != nil {
			return fmt.Errorf("read short during ipv4 read, %v", err)
		}
		host = net.IP(buf[:4]).String()

	case 3:
		if n, err := io.ReadFull(conn, buf[:1]); n != 1 || err != nil {
			return fmt.Errorf("read short during hostname len read, %v", err)
		}
		l := int(buf[0])
		if n, err := io.ReadFull(conn, buf[:l]); n != l || err != nil {
			return fmt.Errorf("read short during hostname read, %v", err)
		}
		host = string(buf[:l])

	case 4:
		if n, err := io.ReadFull(conn, buf[:16]); n != 16 || err != nil {
			return fmt.Errorf("read short during ipv6 read, %v", err)
		}
		host = net.IP(buf[:16]).String()

	default:
		resp[1] = 7
		_, _ = conn.Write(resp)
		return fmt.Errorf("invalid request, %v", buf[:4])
	}

	if n, err := io.ReadFull(conn, buf[:2]); n != 2 || err != nil {
		return fmt.Errorf("read short during port read, %v", err)
	}

	port := strconv.Itoa(int(buf[0])*256 + int(buf[1]))

	address := net.JoinHostPort(host, port)

	upstream, err := p.Dialer(ctx, "tcp", address)
	if err != nil {
		resp[1] = 4
		_, _ = conn.Write(resp)
		return fmt.Errorf("host unreachable, %v", err)
	}
	go func() {
		<-newCtx.Done()
		_ = upstream.Close()
	}()

	if n, err := conn.Write(resp); n != len(resp) || err != nil {
		return fmt.Errorf("write short during response, %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, conn)
		_ = upstream.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, upstream)
		_ = conn.Close()
	}()

	wg.Wait()
	return nil
}
