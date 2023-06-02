package tinyrpc

// 监听--->获取连接--->读取请求--->处理请求--->应答

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
	"tinyrpc/codec"
)

//一个请求的信息包含：头，参数，应答
type request struct {
	h            *codec.Header // header of request
	argv, replyv reflect.Value // argv and replyv of request
	mtype        *methodType
	svc          *service
}

// MagicNumber 魔数通常用于标识RPC协议的版本和类型
const MagicNumber = 0x3bef5c

// Option 在进行正式的RPC请求前，客户端和服务端双方需要确定RPC协议的版本和编解码方式
type Option struct {
	MagicNumber    int
	CodecType      codec.Type
	ConnectTimeout time.Duration // 0 means no limit
	HandleTimeout  time.Duration
}

// DefaultOption 默认的版本和编解码方式
var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: time.Second * 8,
}

// Server 服务端结构体
type Server struct {
	serviceMap sync.Map
}

var DefaultServer = NewServer() // 创建一个默认的服务端
var invalidRequest = struct{}{} //处理请求错误时作为reply

// NewServer 服务器构造函数
func NewServer() *Server {
	return &Server{}
}

// Accept 使用Accept()去接收一个连接
func (server *Server) Accept(lis net.Listener) {
	for {
		//lis.Accept()会阻塞直到接收到建立连接的请求
		//随后创建conn实例，代表着一个连接
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc r: accept error:", err)
			return
		}
		//当拿到连接后，首先要从option中核对各种信息
		//这里交给ServeConn（）处理
		go server.ServeConn(conn)
	}
}

// ServeConn 因为option是选用固定序列化方式（json）编码的，所以直接用json将其解码
//| Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
//| <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc r: options error: ", err)
		return
	}
	//核对魔数
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc r: invalid magic number %x", opt.MagicNumber)
		return
	}
	//根据 CodecType选择解码方式
	//codec.NewCodecFuncMap[opt.CodecType]返回的是一个编码器的构造函数，所以f是一个编码器的构造函数
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc r: invalid codec type %s", opt.CodecType)
		return
	}
	//f(conn)返回的是一个编码器,此步骤是位conn创建一个配置一个解码器和编码器
	cc := f(conn)
	//最后把编码器传进serveCodec（），解析数据
	server.serveCodec(cc, opt)
}
func (server *Server) serveCodec(cc codec.Codec, opt Option) {
	sending := new(sync.Mutex) // make sure to send a complete response，控制
	wg := new(sync.WaitGroup)  // wait until all request are handled
	for {
		//从连接中解析出请求
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection
			}
			//如解析过程中出现错误，将错误信息写进应答，并返回
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		//处理请求，sending, wg用于并发控制，cc用于发送应答。req是请求消息
		go server.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()
	_ = cc.Close()
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {

	//读取头
	h := new(codec.Header)
	err := cc.ReadHeader(h)
	if err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc r: read header error:", err)
		}
		return nil, err
	}
	//根据读取到的头信息创建一个请求
	req := &request{h: h}
	//找到要请求的服务和该服务下的方法
	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()
	// day 1, just suppose it's string
	//常用于动态创建新的变量或对象。此时创建一个string变量
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc r: read body err:", err)
		return req, err
	}
	return req, nil
}

//处理请求
func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()

	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc r: request handle timeout: expect within %s", timeout)
		server.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

//将应答送回
func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Encode(h, body); err != nil {
		log.Println("rpc r: write response error:", err)
	}
}

// Register 注册服务
func (server *Server) Register(rcvr interface{}) error {
	s := creatService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.Split(serviceMethod, ".")
	if len(dot) < 2 {
		err = errors.New("rpc r: service/method request ill-formed: " + serviceMethod)
		return
	}
	//从服务map中根据服务名加载服务
	svci, ok := server.serviceMap.Load(dot[0])
	if !ok {
		err = errors.New("rpc r: can't find service " + dot[0])
		return
	}
	//把svci，一个接口转化为service指针
	svc = svci.(*service)
	//mtype = svci.method[dot[1]]
	mtype = svc.method[dot[1]]
	if mtype == nil {
		err = errors.New("rpc r: can't find method " + dot[1])
	}
	return
}

//暴露给用户方法
func Accept(lis net.Listener)         { DefaultServer.Accept(lis) }
func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }
