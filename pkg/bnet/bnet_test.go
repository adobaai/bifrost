package bnet_test

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adobaai/bifrost/pkg/bnet"
)

func TestNewEncryptedConn(t *testing.T) {
	key := [32]byte{1, 2, 8, 7, 6}
	lis, err := net.Listen("tcp", "")
	require.NoError(t, err)
	serverDone := make(chan struct{})
	go func() {
		serverConn, err := lis.Accept()
		require.NoError(t, err)
		enServerConn, err := bnet.NewEncryptedConn(serverConn, &key, false)
		require.NoError(t, err)
		bs := make([]byte, 10)
		n, err := enServerConn.Read(bs)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), bs[:n])
		serverDone <- struct{}{}
	}()

	clientConn, err := net.Dial(lis.Addr().Network(), lis.Addr().String())
	require.NoError(t, err)
	enClientConn, err := bnet.NewEncryptedConn(clientConn, &key, true)
	require.NoError(t, err)
	n, err := enClientConn.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	<-serverDone
}

func TestVarintFramer(t *testing.T) {
	buf := bytes.Buffer{}
	s := bnet.NewVarintFramer(&buf)
	bs := []byte("hello")
	n, err := s.WriteFrame(bs)
	require.NoError(t, err)
	require.Equal(t, n, len(bs)+1)

	bsr, err := s.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), bsr)

	bs = []byte("xyz")
	n, err = s.WriteFrame(bs)
	require.NoError(t, err)
	require.Equal(t, n, len(bs)+1)

	bsr, err = s.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, []byte("xyz"), bsr)
}

func TestSeqFramer(t *testing.T) {
	buf := bytes.Buffer{}
	f := bnet.NewVarintFramer(&buf)
	sf := bnet.NewSeqFramer(f)
	sf.WriteFrameSeq([]byte("hello"), "write1")
	require.NoError(t, sf.GetError())

	sf.WriteFrameSeq([]byte("hello2"), "write2")
	require.NoError(t, sf.GetError())

	sf.CheckFrameSeq([]byte("hello"), "check1")
	require.NoError(t, sf.GetError())

	sf.CheckFrameSeq([]byte("hello"), "check2")
	require.EqualError(t, sf.GetError(), "check2: not equal")

	sf.WriteFrameSeq([]byte("ok"), "write3")
	require.EqualError(t, sf.GetError(), "check2: not equal")
}
