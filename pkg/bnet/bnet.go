package bnet

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	stream "github.com/nknorg/encrypted-stream"
)

func NewEncryptedConn(conn net.Conn, key *[32]byte, initiator bool) (net.Conn, error) {
	conf := &stream.Config{
		Cipher:          stream.NewXSalsa20Poly1305Cipher(key),
		SequentialNonce: true, // only when key is unique for every stream
		Initiator:       initiator,
	}
	return stream.NewEncryptedStream(conn, conf)
}

// Framer is an interface which read/write one frame at a time.
// This is to solve TCP's sticky packet problem.
type Framer interface {
	ReadFrame() (res []byte, err error)
	WriteFrame(data []byte) (n int, err error)
}

type varintFramer struct {
	rw io.ReadWriter
}

// NewVarintFramer returns a new [Framer]
// that appends a varints encoded length to each frame head.
func NewVarintFramer(rw io.ReadWriter) Framer {
	return &varintFramer{
		rw: rw,
	}
}

// ReadFrame reads a frame.
// The signature of this function is different from [io.Reader],
// so we use a different name to distinguish them.
func (fs *varintFramer) ReadFrame() (res []byte, err error) {
	br := bufio.NewReader(fs.rw)
	length, err := binary.ReadUvarint(br)
	if err != nil {
		return nil, fmt.Errorf("read len: %w", err)
	}

	res = make([]byte, length)
	_, err = io.ReadFull(br, res)
	return
}

func (fs *varintFramer) WriteFrame(p []byte) (n int, err error) {
	// Refer https://protobuf.dev/programming-guides/encoding/#varints:
	//   They allow encoding unsigned 64-bit integers using
	//   anywhere between one and ten bytes, with small values using fewer bytes.
	buf := make([]byte, 0, 10)
	buf = binary.AppendUvarint(buf, uint64(len(p)))
	n, err = fs.rw.Write(buf)
	if err != nil {
		return n, fmt.Errorf("write len: %w", err)
	}

	n2, err := fs.rw.Write(p)
	n += n2
	return
}
