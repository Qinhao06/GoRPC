package server

import (
	"GoRpc/codec"
	"GoRpc/service"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
)

type Server struct {
	serviceMap sync.Map
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

func (s *Server) Register(rcvr interface{}) error {
	server := service.NewService(rcvr)
	if _, ok := s.serviceMap.LoadOrStore(server.Name, server); ok {
		return fmt.Errorf("rpc: service already defined: %v", server.Name)
	}
	return nil
}

func (s *Server) findService(serviceMethod string) (svc *service.Service, mType *service.MethodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		return nil, nil, fmt.Errorf("rpc: service/method request ill-formed: %v", serviceMethod)
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := s.serviceMap.Load(serviceName)
	if !ok {
		return nil, nil, fmt.Errorf("rpc: can't find service %v", serviceName)
	}
	svc = svci.(*service.Service)
	mType = svc.Methods[methodName]
	if mType == nil {
		return nil, nil, fmt.Errorf("rpc: can't find method %v", methodName)
	}
	return svc, mType, nil
}

func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

func (s *Server) HandleAccept(conn net.Conn) {
	log.Println("Server accept conn:", conn)
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
	if option.MagicNumber != codec.MagicNumber {
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
		go s.handleRequest(cc, request, sending, wg)

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

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("Server send response err:", err)
	}
}

func (s *Server) readRequest(cc codec.Codec) (*codec.Request, error) {
	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &codec.Request{H: header}

	req.Svc, req.MType, err = s.findService(header.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.Argv = req.MType.NewArgv()
	req.Replyv = req.MType.NewReplyv()

	argvi := req.Argv.Interface()
	if req.Argv.Type().Kind() != reflect.Ptr {
		argvi = req.Argv.Addr().Interface()
	}
	if err := cc.ReadBody(argvi); err != nil {
		log.Println("Server read request body err:", err)
		return nil, err
	}
	return req, nil

}

func (s *Server) handleRequest(cc codec.Codec, req *codec.Request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := req.Svc.Call(req.MType, req.Argv, req.Replyv)
	if err != nil {
		req.H.Err = err.Error()
		s.sendResponse(cc, req.H, invaildRequest, sending)
	} else {
		s.sendResponse(cc, req.H, req.Replyv.Interface(), sending)
	}
}
