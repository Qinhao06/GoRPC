package client

import (
	"GoRpc/codec"
	"context"
	"io"
	"reflect"
	"sync"
	"time"
)

type XClient struct {
	d          Discovery
	mode       SelectMode
	opt        *codec.Option
	clientsMap map[string]*Client
	mu         *sync.Mutex
}

func (x *XClient) Close() error {
	for key, client := range x.clientsMap {
		err := client.Close()
		if err != nil {
			return err
		}
		delete(x.clientsMap, key)
	}
	return nil
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *codec.Option) *XClient {
	return &XClient{
		d:          d,
		mode:       mode,
		opt:        opt,
		clientsMap: make(map[string]*Client),
		mu:         &sync.Mutex{},
	}
}

func (x *XClient) Dial(rpcAddr string) (*Client, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	client, ok := x.clientsMap[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(x.clientsMap, rpcAddr)
		client = nil
	}
	if client == nil {
		cli, err := XDial(rpcAddr, x.opt)
		if err != nil {
			return nil, err
		}
		x.clientsMap[rpcAddr] = cli
		client = cli
		// time.sleep to wait the option to be handled by server.
		// Because the option is using json for coding.
		// If we don't sleep, the server's json encoding area will swallow subsequent incoming calls with other encoding formats, thereby causing blocking .
		time.Sleep(time.Second)
	}
	return client, nil
}

func (x *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	dial, err := x.Dial(rpcAddr)
	if err != nil {
		return err
	}
	return dial.Call(ctx, serviceMethod, args, reply)
}

func (x *XClient) Call(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	rpcAddr, err := x.d.Get(x.mode)
	if err != nil {
		return err
	}
	return x.call(rpcAddr, ctx, serviceMethod, args, reply)
}

func (x *XClient) Broadcast(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	all, err := x.d.GetAll()
	if err != nil {
		return err
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var e error
	replyDone := reply == nil
	ctx, cancelFunc := context.WithCancel(ctx)
	for _, rpcAddr := range all {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var cloneReply interface{}
			if reply != nil {
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err = x.call(rpcAddr, ctx, serviceMethod, args, reply)
			mu.Lock()
			if err != nil {
				e = err
				cancelFunc()
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	cancelFunc()
	return e
}
