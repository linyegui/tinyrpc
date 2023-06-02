package main

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
	"tinyrpc"
	"tinyrpc/registry"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Product(args Args, reply *int) error {
	*reply = args.Num1 * args.Num2
	return nil
}

func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(registryAddr string, wg *sync.WaitGroup) {
	var foo Foo
	l, _ := net.Listen("tcp", ":0")
	server := tinyrpc.NewServer()
	_ = server.Register(foo)
	//registry.Heartbeat(registryAddr, "tcp@"+l.Addr().String(), 0)
	go registry.RegistryETCD(l.Addr().String())
	fmt.Println("走到这了")
	server.Accept(l)
	_ = http.Serve(l, nil)
	wg.Done()

}

func main() {
	registryAddr := "http://localhost:9999/_tinyrpc_/registry"
	var wg sync.WaitGroup
	wg.Add(2)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	wg.Wait()

}
