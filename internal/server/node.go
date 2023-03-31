package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/multierr"
	"golang.org/x/net/http2"

	"github.com/adobaai/bifrost/pkg/bnet"
	"github.com/adobaai/bifrost/pkg/bnet/hstream"
)

const (
	hbInterval             = 6 * time.Second
	hbMaxConsecutiveErrors = 3

	pathHeartbeat = "H"
)

type Node struct {
	latency       time.Duration
	lastHeartbeat time.Time

	conn    net.Conn
	httpClt *http.Client

	l    logr.Logger
	stop chan struct{}
}

func NewNodeFromConn(conn net.Conn, l logr.Logger) *Node {
	return &Node{
		conn:    conn,
		httpClt: hstream.NewHTTP2ClientFromConn(conn),
		l:       l.WithName("node").WithValues("addr", conn.RemoteAddr()),
		stop:    make(chan struct{}),
	}
}

func (n *Node) Addr() string {
	return n.conn.RemoteAddr().String()
}

func (n *Node) GetID() string {
	return n.Addr()
}

func (n *Node) String() string {
	return fmt.Sprintf("{Lantency:%s,lastHeartbeat:%s}",
		n.latency.Round(time.Millisecond), n.lastHeartbeat.Format(time.RFC3339))
}

// Done returns a channel that's closed when the node is offline.
// This naming convention is derived from the [context.Context] interface.
func (n *Node) Done() <-chan struct{} {
	return n.stop
}

func (n *Node) newStream(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	return hstream.NewClientStream(ctx, n.httpClt, "http:///"+path)
}

/// Handle

func (n *Node) Serve() {
	s := http2.Server{}
	s.ServeConn(n.conn, &http2.ServeConnOpts{
		Handler: http.HandlerFunc(n.handleReq),
	})
}

func (n *Node) handleReq(w http.ResponseWriter, r *http.Request) {
	n.l.V(6).Info("new request", "addr", r.RemoteAddr, "path", r.URL.Path)
	var err error
	defer func() {
		if err != nil {
			n.l.Error(err, "handle req")
		}
	}()

	switch r.URL.Path {
	case "/" + pathHeartbeat: // Heartbeat
		err = n.handleHeartbeat(w, r)
	case "api/nodes":
		_, err = w.Write([]byte("unimplemented"))
	default:
		_, err = w.Write([]byte("invalid request"))
	}
}

/// Heartbeat

func (n *Node) loopHeartbeat() {
	n.l.V(6).Info("loop heartbeat")
	defer n.l.V(6).Info("loop done")

	errCount := 0
	for {
		time.Sleep(hbInterval)
		select {
		case <-n.stop:
			return
		default:
			if err := n.reqHeartbeat(); err == nil {
				errCount = 0
			} else {
				n.l.Error(err, "req heartbeat")
				errCount++
				if errCount == hbMaxConsecutiveErrors {
					n.l.V(4).Info("hbMaxConsecutiveErrors reached, stopping")
					close(n.stop)
					return
				}
			}
		}
	}
}

func (n *Node) reqHeartbeat() (err error) {
	n.l.V(8).Info("req heartbeat")
	defer n.l.V(8).Info("req heartbeat done")

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	s, err := n.newStream(ctx, pathHeartbeat)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer multierr.AppendFunc(&err, s.Close)

	sf := bnet.NewSeqFramer(bnet.NewVarintFramer(s))
	start := time.Now()
	sf.WriteFrameSeq([]byte("hello"), "write hello")
	sf.CheckFrameSeq([]byte("hello2"), "read hello2")
	end := time.Now()
	sf.WriteFrameSeq([]byte("ok"), "write ok")
	if err := sf.GetError(); err != nil {
		return err
	}

	n.latency = end.Sub(start) / 2
	n.lastHeartbeat = end
	return nil
}

func (n *Node) handleHeartbeat(w http.ResponseWriter, r *http.Request) error {
	stream := hstream.NewServerStream(r, w)
	sf := bnet.NewSeqFramer(bnet.NewVarintFramer(stream))
	sf.CheckFrameSeq([]byte("hello"), "read hello")
	start := time.Now()
	sf.WriteFrameSeq([]byte("hello2"), "write hello2")
	sf.CheckFrameSeq([]byte("ok"), "read ok")
	if err := sf.GetError(); err != nil {
		return err
	}

	end := time.Now()
	n.latency = end.Sub(start) / 2
	n.lastHeartbeat = end
	return nil
}
