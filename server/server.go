package server

import (
	"GoRpc/codec"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x3bef5c

var DefaultOption = codec.Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("Server accept err:", err)
			return
		}
		go s.HandleAccept(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

func (s *Server) HandleAccept(conn net.Conn) {
	fmt.Println("Server accept conn:", conn)
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Println("Server conn close err:", err)
		}
	}()
	var option codec.Option
	if err := json.NewDecoder(conn).Decode(&option); err != nil {
		log.Println("Server decode option err:", err)
		return
	}
	if option.MagicNumber != MagicNumber {
		log.Println("Server magic number err")
		return
	}
	f := codec.NewCodecFuncMap[option.CodecType]
	if f == nil {
		log.Println("Server codec type err")
		return
	}

	s.HandleAcceptWithCodec(f(conn))

}

var invaildRequest = struct {
}{}

func (s *Server) HandleAcceptWithCodec(cc codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		request, err := s.readRequest(cc)
		if err != nil {
			if request == nil {
				break
			}
			request.H.Err = err.Error()
			s.sendResponse(cc, request.H, invaildRequest, sending)
		}
		wg.Add(1)
		go s.handleRequest(cc, request, sending, *wg)

	}
	wg.Wait()
	_ = cc.Close()

}

func (s *Server) readRequestHeader(c codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := c.ReadHeader(&h); err != nil {
		log.Println("Server read request header err:", err)
		return nil, err
	}
	return &h, nil
}

func (s *Server) readRequest(cc codec.Codec) (*codec.Request, error) {
	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &codec.Request{H: header}
	req.Argv = reflect.New(reflect.TypeOf(""))
	if err := cc.ReadBody(req.Argv.Interface()); err != nil {
		log.Println("Server read request body err:", err)
		return nil, err
	}
	return req, nil
}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("Server send response err:", err)
	}
}

func (s *Server) handleRequest(cc codec.Codec, req *codec.Request, sending *sync.Mutex, wg sync.WaitGroup) {
	defer wg.Done()
	log.Println(req.H, req.Argv.Elem())
	req.Replyv = reflect.ValueOf(fmt.Sprintf("%d : %s", req.H.Seq, req.Argv.Elem()))
	s.sendResponse(cc, req.H, req.Replyv.Interface(), sending)
}
