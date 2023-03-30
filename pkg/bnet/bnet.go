package bnet

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
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
	r *bufio.Reader
	w io.Writer
}

// NewVarintFramer returns a new [Framer]
// that appends a varints encoded length to each frame head.
func NewVarintFramer(rw io.ReadWriter) Framer {
	return &varintFramer{
		r: bufio.NewReader(rw),
		w: rw,
	}
}

// ReadFrame reads a frame.
// The signature of this function is different from [io.Reader],
// so we use a different name to distinguish them.
func (fs *varintFramer) ReadFrame() (res []byte, err error) {
	length, err := binary.ReadUvarint(fs.r)
	if err != nil {
		return nil, fmt.Errorf("read len: %w", err)
	}

	res = make([]byte, length)
	_, err = io.ReadFull(fs.r, res)
	return
}

func (fs *varintFramer) WriteFrame(p []byte) (n int, err error) {
	// Refer https://protobuf.dev/programming-guides/encoding/#varints:
	//   They allow encoding unsigned 64-bit integers using
	//   anywhere between one and ten bytes, with small values using fewer bytes.
	buf := make([]byte, 0, 10)
	buf = binary.AppendUvarint(buf, uint64(len(p)))
	n, err = fs.w.Write(buf)
	if err != nil {
		return n, fmt.Errorf("write len: %w", err)
	}

	n2, err := fs.w.Write(p)
	n += n2
	return
}

// SeqFramer is a helper that allows you to sequentially read/write bytes
// without checking errors.
// This is not thread-safe.
type SeqFramer struct {
	f   Framer
	err error
}

func NewSeqFramer(f Framer) *SeqFramer {
	return &SeqFramer{
		f: f,
	}
}

func (sf *SeqFramer) GetError() (err error) {
	return sf.err
}

// CheckFrameSeq reads a frame and checks that the data read is equal to the expect bytes.
func (sf *SeqFramer) CheckFrameSeq(expect []byte, msg string) {
	if sf.GetError() != nil {
		return
	}

	bs, err := sf.f.ReadFrame()
	if err == nil && !bytes.Equal(bs, expect) {
		err = errors.New("not equal")
	}
	sf.setError(err, msg)
}

func (sf *SeqFramer) WriteFrameSeq(p []byte, msg string) {
	if sf.GetError() != nil {
		return
	}
	_, err := sf.f.WriteFrame(p)
	sf.setError(err, msg)
}

func (sf *SeqFramer) setError(err error, msg string) {
	if err != nil {
		sf.err = fmt.Errorf("%s: %w", msg, err)
	}
}
