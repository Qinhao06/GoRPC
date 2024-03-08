package main

import (
	"GoRpc/client"
	"GoRpc/server"
	"log"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	// pick a free port
	var foo Foo
	if err := server.Register(&foo); err != nil {
		log.Fatal("service register error:", err)
	}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	server.Accept(l)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	// in fact, following code is like a simple geerpc client
	dial, err := client.Dial("tcp", <-addr)
	if err != nil {
		log.Fatal("network error:", err)
	}
	defer func() {
		if err := dial.Close(); err != nil {
			log.Fatal("network error:", err)
		}
	}()

	time.Sleep(time.Second * 2)
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			if err := dial.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}
