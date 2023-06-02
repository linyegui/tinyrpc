package tinyrpc

import (
	"fmt"
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

//注册method，主要依靠的reflect包，这里需要实现学习一下reflect的一些相关知识点和主义事项

//传入一个实例后，根据这个实例创建一个实例的信息，把这些信息抽象成一个服务
//一个服务其实应答就是一个名称，唯一识别一个服务，还有该服务下的方法
//这里把typ，rcvr这些信息写在结构体内，是方便后序的编码性能。其实也可以去掉，不过性能变差了。
type service struct {
	name   string                 //服务名称，也就是该实例的结构体的名称，我们的请求是的method的格式为Service.Method，此处Service即name
	typ    reflect.Type           //在进行注册时，需要用到
	rcvr   reflect.Value          //结构体本身，在使用f.Call时,需要用到
	method map[string]*methodType //用map存储该结构导出的方法，方法名以及方法
}

//描述远程调用的方法信息，以便于编解码和网络传输。
type methodType struct {
	//方法本身
	method reflect.Method
	//方法的第一个参数的类型
	ArgType reflect.Type
	//方法的第二个参数的的类型
	ReplyType reflect.Type
	//方法被调用的次数
	numCalls uint64
}

type method struct {
	//方法本身
	method reflect.Value
	//方法的第一个参数的类型
	ArgType reflect.Type
	//方法的第二个参数的的类型
	ReplyType reflect.Type
	//方法被调用的次数
	numCalls uint64
}

//newService()实现的是创建一个服务(通常一个服务下，有多个method)，method存在service下的一个map中
func NewService(rcvr interface{}) *service {
	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = s.typ.Name()
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc r: %s is not a valid service name", s.name)
	}
	//将rcvr的所有方法写到method map[string]*methodType
	s.registerMethods()
	return s
}

//
func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		//log.Printf("rpc r: register %s.%s\n", s.name, method.Name)
	}
}

//判断是否可导出
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

//f.Call() 是使用反射调用函数f，并传递函数参数的方法。
//它接收一个反射值切片作为参数，其中包含了函数调用时需要传递的所有参数。
//函数调用的返回值也是一个反射值切片。在这个例子中， 它被用于调用一个方法，
//并传递了三个参数：接收器（即该方法所属的结构体实例）、 包含调用该方法时传递的参数的切片和用于存储方法调用返回值的变量的反射值。
func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func //获取一个函数
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}

func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	// arg may be a pointer type, or a value type

	if m.ArgType.Kind() == reflect.Ptr {
		//m.ArgType是指针的话.Elem()获取实际类型，new创建了一个指向该类型实例的指针，返回指针
		argv = reflect.New(m.ArgType.Elem())
	} else {
		//m.ArgType不是是指针的话，new创建了该类型实例的指针，Elem()返回指针的实际类型
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m *methodType) newReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

func creatService(rcvr interface{}) *service {
	s := new(service)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = reflect.TypeOf(rcvr).Name()
	if !ast.IsExported(s.name) {
		log.Fatalf("222rpc r: %s is not a valid service name", s.name)
	}
	s.method = make(map[string]*methodType)

	for i := 0; i < reflect.TypeOf(rcvr).NumMethod(); i++ {
		method := reflect.TypeOf(rcvr).Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		//log.Printf("rpc r: register %s.%s\n", s.name, method.Name)
	}
	return s
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}
