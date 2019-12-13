package bfe_grpc

import (
	"context"
	"google.golang.org/grpc"
	"errors"
)

var (
	ErrExtractMetadata = errors.New("error extracting metadata from request")

	ErrUnknownMethodString = "unknown method"
)

type StreamDirector func(ctx context.Context, methodName string) (context.Context, *grpc.ClientConn, error)

func Director() func(ctx context.Context, methodName string) (context.Context, *grpc.ClientConn, error) {

	return func(ctx context.Context, methodName string) (context.Context, *grpc.ClientConn, error) {
		// TODO: tls
		// TODO: balance
		// TODO: unknown method
		conn, err := grpc.DialContext(ctx, "localhost:50000", grpc.WithCodec(Codec()), grpc.WithInsecure())
		return ctx, conn, err
	}
}