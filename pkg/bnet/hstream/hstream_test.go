package hstream_test

import (
	"net"
	"net/http"
	"testing"

	"golang.org/x/net/http2"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/adobaai/bifrost/pkg/bnet/hstream"
)

func TestStream(t *testing.T) {
	gl := testr.NewWithOptions(t, testr.Options{
		Verbosity: 9,
	})
	lis, err := net.Listen("tcp", "")
	require.NoError(t, err)
	gl.V(4).Info("listening", "addr", lis.Addr())

	go func() {
		l := gl.WithName("server")
		for {
			conn, err := lis.Accept()
			l.V(6).Info("accepted", "remoteAddr", conn.RemoteAddr())
			require.NoError(t, err)
			server := http2.Server{}
			server.ServeConn(conn, &http2.ServeConnOpts{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					s := hstream.NewServerStream(r, w)
					bs := make([]byte, 10)
					n, err := s.Read(bs)
					require.NoError(t, err)
					assert.Equal(t, 5, n)
					assert.Equal(t, []byte("hello"), bs[:n])

					n, err = s.Write([]byte("hello2"))
					require.NoError(t, err)
					assert.Equal(t, n, 6)
				}),
			})
		}
	}()

	conn, err := net.Dial("tcp", lis.Addr().String())
	require.NoError(t, err)
	clt := hstream.NewHTTP2ClientFromConn(conn)
	cs, err := hstream.NewClientStream(clt, "http://yes.com")
	require.NoError(t, err)
	n, err := cs.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	bs := make([]byte, 10)
	n, err = cs.Read(bs)
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, []byte("hello2"), bs[:n])

	assert.NoError(t, cs.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, lis.Close())
}
