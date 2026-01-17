package plugins

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

type GRPCServer interface {
	OnConnect(context.Context, *ConnectRequest) (*ConnectResponse, error)
}

func RegisterGRPCServer(s *grpc.Server, impl GRPCServer) {
	encoding.RegisterCodec(jsonCodec{})
	service := &grpc.ServiceDesc{
		ServiceName: "hyrouter.Plugin",
		HandlerType: (*GRPCServer)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "OnConnect", Handler: onConnectHandler(impl)},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "",
	}
	s.RegisterService(service, impl)
}

func onConnectHandler(impl GRPCServer) func(any, context.Context, func(any) error, grpc.UnaryServerInterceptor) (any, error) {
	return func(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		var req ConnectRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		return impl.OnConnect(ctx, &req)
	}
}
