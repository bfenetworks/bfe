package bfe_grpc

import (
	"fmt"
)

import (
	"google.golang.org/grpc"
)

func Codec() grpc.Codec {
	return DefaultCodec(&protoCodec{})
}

func DefaultCodec(fallback grpc.Codec) grpc.Codec {
	return &rawCodec{fallback}
}

type rawCodec struct {
	deaultCodec grpc.Codec
}

type frame struct {
	payload []byte
}

func (c *rawCodec) String() string {
	return fmt.Sprintf("proxy>%s", c.deaultCodec.String())
}

func (c *rawCodec) Marshal(v interface{}) ([]byte, error) {
	out, ok := v.(*frame)
	if !ok {
		return c.deaultCodec.Marshal(v)
	}
	return out.payload, nil

}

func (c *rawCodec) Unmarshal(data []byte, v interface{}) error {
	dst, ok := v.(*frame)
	if !ok {
		return c.deaultCodec.Unmarshal(data, v)
	}
	dst.payload = data
	return nil
}