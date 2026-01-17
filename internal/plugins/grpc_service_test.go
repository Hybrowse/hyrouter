package plugins

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

type dummyServer struct{}

func (d *dummyServer) OnConnect(context.Context, *ConnectRequest) (*ConnectResponse, error) {
	return &ConnectResponse{}, nil
}

func TestOnConnectHandler_DecodeError(t *testing.T) {
	h := onConnectHandler(&dummyServer{})
	_, err := h(nil, context.Background(), func(any) error { return context.Canceled }, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestOnConnectHandler_Success(t *testing.T) {
	h := onConnectHandler(&dummyServer{})
	_, err := h(nil, context.Background(), func(v any) error {
		req := v.(*ConnectRequest)
		req.Event.SNI = "x"
		return nil
	}, grpc.UnaryServerInterceptor(nil))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}
