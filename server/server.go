package server

import (
	"GoRpc/codec"
	"GoRpc/service"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer *Server

func init() {
	DefaultServer = NewServer()
}

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
	newService := service.NewService(rcvr)
	if _, ok := s.serviceMap.LoadOrStore(newService.Name, newService); ok {
		return fmt.Errorf("rpc: service already defined: %v", newService.Name)
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
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Printf("Server %p conn close err:%s\n", s, err)
		}
	}()
	var option codec.Option
	if err := json.NewDecoder(conn).Decode(&option); err != nil {
		log.Printf("Server %p decode option err: %s\n", s, err)
		return
	}
	if option.MagicNumber != codec.MagicNumber {
		log.Printf("Server %p magic number err\n", s)
		return
	}
	f := codec.NewCodecFuncMap[option.CodecType]
	if f == nil {
		log.Printf("Server %p codec type err\n", s)
		return
	}
	log.Println(fmt.Sprintf("Server %p accept %+v, opt:%+v", s, conn, option))
	s.HandleAcceptWithCodec(f(conn), &option)

}

var invalidRequest = struct {
}{}

func (s *Server) HandleAcceptWithCodec(cc codec.Codec, option *codec.Option) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		request, err := s.readRequest(cc)
		if err != nil {
			if request == nil {
				break
			}
			request.H.Err = err.Error()
			s.sendResponse(cc, request.H, invalidRequest, sending)
		}
		wg.Add(1)
		log.Println(fmt.Sprintf("Server %p receive req: %+v %+v", s, request, cc))
		go s.handleRequest(cc, request, sending, wg, option.HandleTimeOut)

	}
	wg.Wait()
	_ = cc.Close()

}

func (s *Server) readRequestHeader(c codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := c.ReadHeader(&h); err != nil {
		log.Printf("Server %p read request header err:%s\n", s, err)
		return nil, err
	}
	return &h, nil
}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Printf("Server %p send response err: %s\n", s, err)
	}
}

func (s *Server) readRequest(cc codec.Codec) (*codec.Request, error) {
	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &codec.Request{H: header}

	req.Svc, req.MType, err = s.findService(header.ServiceMethod)
	if req.MType == nil {
		log.Printf("Server %p find service err: %+v\n %+v", s, req, header)
		return req, fmt.Errorf("rpc: can't find method %v", header.ServiceMethod)
	}
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
		log.Printf("Server %p read request body err:%s\n", s, err)
		return nil, err
	}
	return req, nil

}

func (s *Server) handleRequest(cc codec.Codec, req *codec.Request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()

	callChan := make(chan struct{}, 1)
	sentChan := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		err := req.Svc.Call(req.MType, req.Argv, req.Replyv)
		callChan <- struct{}{}
		select {
		case <-ctx.Done():
			return
		default:

		}
		if err != nil {
			req.H.Err = err.Error()
			s.sendResponse(cc, req.H, invalidRequest, sending)
		} else {
			s.sendResponse(cc, req.H, req.Replyv.Interface(), sending)
		}

		sentChan <- struct{}{}
	}()

	if timeout == 0 {
		<-callChan
		<-sentChan
		return
	}
	select {
	// handle call timeout
	case <-time.After(timeout):
		req.H.Err = fmt.Sprintf("request handle timeout timeout: %s", timeout)
		s.sendResponse(cc, req.H, invalidRequest, sending)
	case <-callChan:
		// wait for send
		<-sentChan
	}
}

/*
 HTTP
*/

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = fmt.Fprintf(w, "405 must CONNECT\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("Server hijack", r.RemoteAddr, " err:", err)
		return
	}
	_, _ = fmt.Fprintf(conn, "HTTP/1.0 "+codec.Connected+"\n\n")
	s.HandleAccept(conn)
}

func (s *Server) HandleHttp() {
	itoa := strconv.Itoa(rand.Intn(10))
	http.Handle(codec.DefaultRpcPath+"/"+itoa, s)
	log.Println("rpc server defaultRpcPath:", codec.DefaultRpcPath, itoa)
	http.Handle(codec.DefaultDebugPath+"/"+itoa, debugHttp{s})
	log.Printf("rpc server %p debug path: %s\n", s, codec.DefaultDebugPath+"/"+itoa)
}

func HandleHTTP() {
	DefaultServer.HandleHttp()
}

var _ http.Handler = (*Server)(nil)
