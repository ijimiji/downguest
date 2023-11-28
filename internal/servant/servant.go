package servant

import (
	"context"
	"fmt"
	"net"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
)

type Config struct {
	Port string
}

func New(config Config) (*servant, error) {
	return &servant{}, nil
}

type servant struct {
	Name     string
	handlers map[string]grpcServiceHandler
}

func (s *servant) Serve() error {
	server := grpc.NewServer()
	// server.RegisterService()

	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		return err
	}

	return server.Serve(l)
}

type handler = func(context.Context, proto.Message) (proto.Message, error)
type grpcServiceHandler = func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error)

func HandleFunc[I proto.Message](s *servant, method string, handler handler) {
	s.handlers[method] = func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		var in I
		if err := dec(in); err != nil {
			return nil, err
		}
		if interceptor == nil {
			return handler(ctx, in)
		}
		info := &grpc.UnaryServerInfo{
			Server:     srv,
			FullMethod: fmt.Sprintf("/%s/%s", s.Name, method),
		}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return handler(ctx, req.(I))
		}
		return interceptor(ctx, in, info, handler)
	}
	// 	var request I
	// proto.Unmarshal(nil, request)
}
