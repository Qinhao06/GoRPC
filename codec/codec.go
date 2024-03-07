package codec

import (
	"io"
)

type Header struct {
	ServerMethod string // 服务端方法名
	Seq          uint64 // 序列号
	Err          string // 错误信息
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
	ReadRequest(interface{}) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

const (
	JsonType  Type = "application/json"
	ProtoType Type = "application/protobuf"
	GobType   Type = "application/gob"
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
