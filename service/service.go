package service

import (
	"fmt"
	"go/ast"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
)

type MethodType struct {
	method     reflect.Method
	ArgType    reflect.Type
	ReplyType  reflect.Type
	NumberCall uint64
	mu         *sync.Mutex
}

func (m *MethodType) NumberCalls() uint64 {
	return atomic.LoadUint64(&m.NumberCall)
}

func (m *MethodType) NewArgv() reflect.Value {
	var argv reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m *MethodType) NewReplyv() reflect.Value {
	var replyv reflect.Value
	replyv = reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

type Service struct {
	Name    string
	Methods map[string]*MethodType
	Typ     reflect.Type
	Rcvr    reflect.Value
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *Service) registerMethods() {
	s.Methods = make(map[string]*MethodType)
	for m := 0; m < s.Typ.NumMethod(); m++ {
		method := s.Typ.Method(m)
		mType := method.Type
		mName := method.Name
		if !ast.IsExported(mName) {
			continue
		}
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		replyType := mType.In(2)
		argType := mType.In(1)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.Methods[mName] = &MethodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
			mu:        &sync.Mutex{},
		}
		log.Println(fmt.Sprintf("Service %p : register Service: %s method: %s", s, s.Name, mName))
	}
}

func (s *Service) Call(m *MethodType, argv reflect.Value, replyv reflect.Value) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddUint64(&m.NumberCall, 1)
	f := m.method.Func
	callReturns := f.Call([]reflect.Value{s.Rcvr, argv, replyv})
	log.Println(fmt.Sprintf("Service %p call Service: %s.%s", s, s.Name, m.method.Name))
	if err := callReturns[0].Interface(); err != nil {
		return err.(error)
	}
	return nil
}

func NewService(Rcvr interface{}) *Service {
	s := new(Service)
	s.Rcvr = reflect.ValueOf(Rcvr)
	s.Typ = reflect.TypeOf(Rcvr)
	s.Name = reflect.Indirect(s.Rcvr).Type().Name()
	if !ast.IsExported(s.Name) {
		log.Fatalf("Service: %s is not a valid Service Name", s.Name)
	}
	s.registerMethods()
	return s
}
