package bfe_grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc"
)

// PreparedMsg is responsible for creating a Marshalled and Compressed object.
type PreparedMsg struct {
	// Struct for preparing msg before sending them
	encodedData []byte
	hdr         []byte
	payload     []byte
}

// Encode marshalls and compresses the message using the codec and compressor for the stream.
func (p *PreparedMsg) Encode(s grpc.Stream, msg interface{}) error {
	ctx := s.Context()
	rpcInfo, ok := rpcInfoFromContext(ctx)
	if !ok {
		return status.Errorf(codes.Internal, "bfe_grpc: unable to get rpcInfo")
	}

	// check if the context has the relevant information to prepareMsg
	if rpcInfo.preloaderInfo == nil {
		return status.Errorf(codes.Internal, "bfe_grpc: rpcInfo.preloaderInfo is nil")
	}
	if rpcInfo.preloaderInfo.codec == nil {
		return status.Errorf(codes.Internal, "bfe_grpc: rpcInfo.preloaderInfo.codec is nil")
	}

	// prepare the msg
	data, err := encode(rpcInfo.preloaderInfo.codec, msg)
	if err != nil {
		return err
	}
	p.encodedData = data
	compData, err := compress(data, rpcInfo.preloaderInfo.cp, rpcInfo.preloaderInfo.comp)
	if err != nil {
		return err
	}
	p.hdr, p.payload = msgHeader(data, compData)
	return nil
}
