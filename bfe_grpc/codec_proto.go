package bfe_grpc

import (
	"github.com/golang/protobuf/proto"
)

// Codec with proto
type protoCodec struct{}

func (protoCodec) String() string {
	return "proto"
}

func (protoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (protoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}