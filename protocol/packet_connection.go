package protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

type PacketConnection struct {
	bb *bufio.ReadWriter
	net.Conn
}

func NewPacketConnection(conn net.Conn) *PacketConnection {
	r := bufio.NewReaderSize(conn, 65538)
	w := bufio.NewWriterSize(conn, 65538)
	return &PacketConnection{bufio.NewReadWriter(r, w), conn}
}

func (pc *PacketConnection) Read(buf []byte) (int, error) {
	if len(buf) < 65536 {
		return 0, errors.New("too small buffer for packet connection")
	}
	var binLen [2]byte
	if _, err := io.ReadFull(pc.Conn, binLen[:]); err != nil {
		return 0, err
	}

	packetLen := int(binary.BigEndian.Uint16(binLen[:]))
	return io.ReadFull(pc.Conn, buf[:packetLen])
}

func (pc *PacketConnection) Write(buf []byte) (int, error) {
	if len(buf) > 65536 {
		return 0, errors.New("too big packet for packet connection")
	}

	var binLen [2]byte
	binary.BigEndian.PutUint16(binLen[:], uint16(len(buf)))
	if _, err := pc.bb.Write(binLen[:]); err != nil {
		return 0, err
	}

	if _, err := pc.bb.Write(buf); err != nil {
		return 0, err
	}

	if err := pc.bb.Flush(); err != nil {
		return 0, err
	}

	return len(buf), nil
}
