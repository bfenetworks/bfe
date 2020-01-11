package bfe_grpc

import (
	"context"
	"errors"
)

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	ErrExtractMetadata = errors.New("error extracting metadata from request")

	ErrUnknownMethodString = "unknown method"
)

// StreamDesc
type StreamDirector func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error)

func Director() func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {

	return func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		// TODO: balance
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return ctx, nil, ErrExtractMetadata
		}

		outCtx := metadata.NewOutgoingContext(ctx, md.Copy())

		conn, err := grpc.DialContext(outCtx, "localhost:50000", grpc.WithCodec(CustomCodec()), grpc.WithInsecure())
		return outCtx, conn, err
	}
}