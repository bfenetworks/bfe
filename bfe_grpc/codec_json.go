package bfe_grpc

import (
	"encoding/json"
)

// Codec with json
type jsonCodec struct{}

func (jsonCodec) String() string {
	return "json"
}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
