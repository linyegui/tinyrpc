package main

import (
	"context"
	"log"
	"sync"
	"time"
	"tinyrpc"
	"tinyrpc/xclient"
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

func foo(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(xc *xclient.XClient) {
	defer func() { _ = xc.Close() }()
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			foo(xc, context.Background(), "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})

		}(i)
	}
	wg.Wait()
}

func broadcast(xc *xclient.XClient) {

	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			foo(xc, ctx, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	d, err := xclient.NewETCD(make([]string, 0), 0)
	if err != nil {
		log.Printf("创建连接:" + err.Error())
	}
	go xclient.LoadAndFresh(d)
	opt := tinyrpc.Option{
		CodecType: "application/json",
	}
	xc := xclient.NewXClient(d, xclient.RandomSelect, &opt)
	defer func() { _ = xc.Close() }()
	time.Sleep(1 * time.Second)
	for {
		call(xc)
		broadcast(xc)
		time.Sleep(2 * time.Second)
	}

}
