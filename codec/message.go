package codec

import "reflect"

type Option struct {
	MagicNumber uint32 // magic number
	CodecType   Type
}

type Request struct {
	H            *Header
	Argv, Replyv reflect.Value
}
