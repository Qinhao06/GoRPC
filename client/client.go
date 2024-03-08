package client

import (
	"GoRpc/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}
type Client struct {
	cc       codec.Codec
	opt      *codec.Option
	sending  *sync.Mutex // 写锁
	header   codec.Header
	mu       *sync.Mutex // 修改 client 状态锁
	seq      uint64
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.cc.Close()
}

func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shutdown || c.closing {
		return 0, ErrShutdown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.Done <- call
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case h.Err != "":
			call.Error = fmt.Errorf(h.Err)
			err = c.cc.ReadBody(nil)
			call.Done <- call
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body err:" + err.Error())
			}
			call.Done <- call
		}

	}
}

func (c *Client) send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()

	callSeq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.Done <- call
		return
	}

	c.header.Seq = callSeq
	c.header.ServiceMethod = call.ServiceMethod
	c.header.Err = ""

	if err = c.cc.Write(&c.header, call.Args); err != nil {
		call := c.removeCall(callSeq)
		if call != nil {
			call.Error = err
			call.Done <- call
		}
	}
}

func (c *Client) DoCall(serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	c.send(call)
	return call

}

func (c *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
	call := <-c.DoCall(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}

func NewClient(conn net.Conn, opt *codec.Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("unsupported codec type %v\n", opt.CodecType)
		return nil, fmt.Errorf("unsupported codec type %v", opt.CodecType)
	}
	cc := f(conn)

	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("json encode option err:", err)
		_ = cc.Close()
		return nil, err
	}
	client := &Client{
		cc:       cc,
		opt:      opt,
		sending:  &sync.Mutex{},
		mu:       &sync.Mutex{},
		seq:      1, // 如果seq = 0表示请求失败
		pending:  make(map[uint64]*Call),
		shutdown: false,
	}
	go client.receive()
	return client, nil
}

func parseOptions(opts ...*codec.Option) (*codec.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return &codec.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = codec.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = codec.DefaultOption.CodecType
	}
	return opt, nil
}

func Dial(network, address string, opts ...*codec.Option) (client *Client, err error) {
	option, err := parseOptions(opts...)
	if err != nil {
		log.Println("parse options err:", err)
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		log.Println("net dial err:", err)
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, option)
}
