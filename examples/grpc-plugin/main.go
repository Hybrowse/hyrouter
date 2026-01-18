//go:build examples

package main

import (
	"context"
	"flag"
	"net"
	"strings"

	"github.com/hybrowse/hyrouter/internal/plugins"
	"github.com/hybrowse/hyrouter/internal/routing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type server struct{}

func (s *server) OnConnect(ctx context.Context, req *plugins.ConnectRequest) (*plugins.ConnectResponse, error) {
	_ = ctx
	resp := &plugins.ConnectResponse{}

	if strings.EqualFold(req.Event.Username, "deny") {
		resp.Deny = true
		resp.DenyReason = "denied by grpc plugin"
		return resp, nil
	}

	if strings.Contains(strings.ToLower(req.Event.SNI), "grpc") {
		resp.Backend = &routing.Backend{Host: "play.hyvane.com", Port: 5520}
	}

	resp.ReferralData = []byte("grpc-plugin")
	return resp, nil
}

func main() {
	listen := flag.String("listen", "127.0.0.1:7777", "listen address")
	flag.Parse()

	l, err := net.Listen("tcp", *listen)
	if err != nil {
		panic(err)
	}

	s := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	plugins.RegisterGRPCServer(s, &server{})
	if err := s.Serve(l); err != nil {
		panic(err)
	}
}
