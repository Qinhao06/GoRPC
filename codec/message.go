package codec

import (
	"GoRpc/service"
	"reflect"
	"time"
)

type Option struct {
	MagicNumber    uint32 // magic number
	CodecType      Type
	ConnectTimeOut time.Duration
	HandleTimeOut  time.Duration
}

type Request struct {
	H            *Header
	Argv, Replyv reflect.Value
	MType        *service.MethodType
	Svc          *service.Service
}

const MagicNumber = 0x3bef5c

var DefaultOption = Option{
	MagicNumber:    MagicNumber,
	CodecType:      GobType,
	ConnectTimeOut: 10 * time.Second,
}

const (
	Connected        = "200 Connected to GoRpc"
	DefaultRpcPath   = "/_goRPC_"
	DefaultDebugPath = "/goRpc/debug"
)
