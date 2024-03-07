package client

import (
	"GoRpc/codec"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan Call
}
type Client struct {
	cc  codec.GobCodec
	opt codec.Option
}
