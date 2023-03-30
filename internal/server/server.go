package server

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/sourcegraph/conc"
	"go.uber.org/multierr"

	"github.com/adobaai/bifrost/internal/config"
	"github.com/adobaai/bifrost/pkg/bnet"
)

type Server struct {
	conf *config.Config
	l    logr.Logger

	nodes sync.Map
	lis   net.Listener
	stop  chan struct{}
}

func New(conf *config.Config, l logr.Logger) *Server {
	return &Server{
		conf: conf,
		l:    l,
		stop: make(chan struct{}),
	}
}

func (s *Server) Start() (err error) {
	lis, err := net.Listen("tcp", s.conf.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.l.V(4).Info("server listening", "addr", lis.Addr())
	s.lis = lis

	var wg conc.WaitGroup
	// If an error occurs while accepting,
	// the program will wait for all goroutines to complete before exiting.
	acceptErr := make(chan error)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				acceptErr <- err
				return
			}

			s.l.V(6).Info("new conn", "remoteAddr", conn.RemoteAddr())
			wg.Go(func() {
				s.handleConn(conn)
			})
		}
	}()
	go func() {
		for _, addr := range s.conf.Nodes {
			if err := s.initNode(addr); err != nil {
				s.l.Error(err, "init node", "addr", addr)
			}
		}
	}()

	select {
	case <-s.stop:
	case err := <-acceptErr:
		multierr.AppendInto(&err, fmt.Errorf("accept: %w", err))
	}
	multierr.AppendFunc(&err, s.lis.Close)
	wg.Wait()
	return
}

// Stop stops the server, which is not thread-safe.
func (s *Server) Stop() error {
	close(s.stop)
	return nil
}

/// Nodes

func (s *Server) addNode(n *Node) {
	s.setNode(n.GetID(), n)
}

func (s *Server) setNode(id string, n *Node) {
	s.nodes.Store(id, n)
}

func (s *Server) delNode(n *Node) {
	s.nodes.Delete(n.GetID())
}

/// Connection

// handleConn handles the incoming connection.
func (s *Server) handleConn(conn net.Conn) {
	l := s.l.WithValues("addr", conn.RemoteAddr())
	enConn, err := bnet.NewEncryptedConn(conn, s.conf.SecretArray(), false)
	if err != nil {
		l.Error(err, "new encrypted conn")
		return
	}

	go func() {
		<-s.stop
		conn.Close()
	}()

	n := NewNodeFromConn(enConn, s.l)
	s.addNode(n)
	n.Serve()
	l.Info("node offline")
	s.delNode(n)
}

func (s *Server) initNode(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	conn, err = bnet.NewEncryptedConn(conn, s.conf.SecretArray(), true)
	if err != nil {
		return fmt.Errorf("new encrypted conn: %w", err)
	}

	n := NewNodeFromConn(conn, s.l)
	if err := n.reqHeartbeat(); err != nil {
		return fmt.Errorf("first heartbeat: %w", err)
	}

	s.addNode(n)
	go func() {
		<-n.Done()
		s.delNode(n)
		s.l.V(4).Info("node offline", "addr", n.Addr())
	}()

	go n.loopHeartbeat()
	return nil
}
