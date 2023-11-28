package main

import (
	"context"
	"downguest/proto"
	"fmt"
	"log"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

type server struct {
	proto.UnimplementedEchoServer
}

// SayHello implements helloworld.GreeterServer
func (s *server) Serve(ctx context.Context, in *proto.TContext) (*proto.TContext, error) {

	httpResponse, err := anypb.New(&proto.THTTPResponse{
		Body: "hello",
	})
	if err != nil {
		return nil, err
	}

	resp := &proto.TContext{
		Data: map[string]*anypb.Any{
			"http_response": httpResponse,
		},
	}
	return resp, nil
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 8081))
	if err != nil {
		slog.Error("connect to port")
	}
	s := grpc.NewServer()
	proto.RegisterEchoServer(s, new(server))
	log.Fatal(s.Serve(lis))
}
