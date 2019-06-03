package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"io"
	"net"
)

func ReadRequest(ctx context.Context, from net.Conn) (*ConnectionRequest, error) {
	cr := &ConnectionRequest{}
	if err := readPacket(ctx, from, cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func ReadResponse(ctx context.Context, from net.Conn) (*ConnectionResponse, error) {
	cr := &ConnectionResponse{}
	if err := readPacket(ctx, from, cr); err != nil {
		return nil, err
	}
	return cr, nil
}

func WritePacket(ctx context.Context, to net.Conn, val interface{}) error {
	buf := bytes.NewBuffer([]byte{0, 0, 0, 0})
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(val); err != nil {
		return err
	}

	l := buf.Len() - 4
	data := buf.Bytes()
	binary.BigEndian.PutUint32(data[:4], uint32(l))

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-newCtx.Done()
		if ctx.Err() != nil {
			_ = to.Close()
		}
	}()

	_, err := to.Write(data)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return err
}

func readPacket(ctx context.Context, from net.Conn, val interface{}) error {
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-newCtx.Done()
		if ctx.Err() != nil {
			_ = from.Close()
		}
	}()

	checkContext := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	var l [4]byte
	if _, err := io.ReadFull(from, l[:]); err != nil {
		return checkContext(err)
	}

	structLen := binary.BigEndian.Uint32(l[:])
	data := make([]byte, structLen)
	if _, err := io.ReadFull(from, data); err != nil {
		return checkContext(err)
	}

	decoder := gob.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(val); err != nil {
		return err
	}
	return nil
}
