package tinyrpc

import (
	"testing"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Bum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Cum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Dum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Eum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func BenchmarkCreatService(b *testing.B) {
	var a Foo
	var rcvr interface{}
	rcvr = a
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		creatService(rcvr)
	}
}

func BenchmarkNewService(b *testing.B) {
	var a Foo
	var rcvr interface{}
	rcvr = a
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewService(rcvr)
	}
}
