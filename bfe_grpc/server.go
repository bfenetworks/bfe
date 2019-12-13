package bfe_grpc

import (
	"net"
	"context"
)

import (
	"github.com/baidu/bfe/bfe_balance"
)

import (
	"google.golang.org/grpc"
)

type Server struct {
	//TODO: args
	server *grpc.Server
	config *Config
	balTable   *bfe_balance.BalTable
}

func (s *Server) Serve(ln net.Listener) error {
	return s.server.Serve(ln)
	}

func (s *Server) Stop() {
	s.server.Stop()
}

func (s *Server) GracefulStop(ctx context.Context) {
	s.server.GracefulStop()
}


