package codec

import (
	"GoRpc/service"
	"reflect"
)

type Option struct {
	MagicNumber uint32 // magic number
	CodecType   Type
}

type Request struct {
	H            *Header
	Argv, Replyv reflect.Value
	MType        *service.MethodType
	Svc          *service.Service
}

const MagicNumber = 0x3bef5c

var DefaultOption = Option{
	MagicNumber: MagicNumber,
	CodecType:   GobType,
}
