// Package hstream implements a read-write stream using HTTP.
package hstream

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"go.uber.org/multierr"
	"golang.org/x/net/http2"
)

// NewHTTP2ClientFromConn returns a new HTTP2 client
// which send requests using the provided conn.
//
// Refer https://github.com/thrawn01/h2c-golang-example.
func NewHTTP2ClientFromConn(conn net.Conn) *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			// So http2.Transport doesn't complain the URL scheme isn't 'https'
			AllowHTTP: true,
			// Pretend we are dialing a TLS endpoint.
			// (Note, we ignore the passed tls.Config)
			DialTLS: func(_, _ string, _ *tls.Config) (net.Conn, error) {
				return conn, nil
			},
		},
	}
}

type serverStream struct {
	req  *http.Request
	resp http.ResponseWriter
}

// NewServerStream constructs an [io.ReadWriter] from the incoming request.
func NewServerStream(r *http.Request, w http.ResponseWriter) io.ReadWriter {
	return &serverStream{
		req:  r,
		resp: w,
	}
}

func (ss *serverStream) Read(p []byte) (n int, err error) {
	// There is no need to close the req.Body, as doc said:
	//   The Server will close the request body. The ServeHTTP
	//   Handler does not need to.
	return ss.req.Body.Read(p)
}

func (ss *serverStream) Write(p []byte) (n int, err error) {
	n, err = ss.resp.Write(p)
	// It's important to flush the data immediately, or else the client won't be able
	// to receive it and the stream will become unresponsive.
	// This is the first major bug in development.
	if f, ok := ss.resp.(http.Flusher); ok {
		f.Flush()
	}
	return
}

type clientStream struct {
	pr   *io.PipeReader
	pw   *io.PipeWriter
	resp *http.Response
	// doErr is error returned by [http.Client.Do].
	// It is worth noting that this field is not thread-safe,
	// although this is not a concern
	// as the affected fields (pr and pw) are already thread-safe.
	doErr error
	// This channel is closed when the [http.Client.Do] is returned
	done chan struct{}
}

// NewClientStream constructs an [io.ReadWriteCloser] to the specified URL.
func NewClientStream(clt *http.Client, url string) (res io.ReadWriteCloser, err error) {
	pr, pw := io.Pipe()
	s := clientStream{
		pr:   pr,
		pw:   pw,
		done: make(chan struct{}),
	}
	req, err := http.NewRequest("POST", url, pr)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	go func() {
		resp, err := clt.Do(req)
		if err != nil {
			s.doErr = err
			// Without closing, the program will block on the write method.
			// This is the third major bug in development.
			s.pw.Close()
		}

		s.resp = resp
		close(s.done)
	}()

	return &s, nil
}

func (cs *clientStream) Read(p []byte) (n int, err error) {
	<-cs.done
	if err := cs.doErr; err != nil {
		return 0, err
	}
	return cs.resp.Body.Read(p)
}

func (cs *clientStream) Write(p []byte) (n int, err error) {
	if err := cs.doErr; err != nil {
		return 0, err
	}
	return cs.pw.Write(p)
}

func (cs *clientStream) Close() (err error) {
	err = cs.pw.Close()
	// HACK To ensure the pipe is fully drained before closing the body,
	// we need to include a sleep function here.
	// Without this, the data in the code below may not be sent to the counterpart
	// before closing.
	//   cs.Write(data)
	//   cs.Close()
	//
	// This is the second major bug in development.
	time.Sleep(time.Millisecond)
	if cs.resp != nil {
		multierr.AppendInto(&err, cs.resp.Body.Close())
	}
	return
}
