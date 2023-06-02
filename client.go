package tinyrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"tinyrpc/codec"
)

//发起连接--->发送请求--->记录请求状态--->接收请求--->修改请求状态--->关闭连接

// Call 包含一次远程调用的所有信息，
type Call struct {
	Seq           uint64
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // arguments to the function
	Reply         interface{} // reply from the function
	Error         error       // if error occurs, it will be set
	Done          chan *Call  // Strobes when call is complete.
}

// Client 客户端的的结构体
type Client struct {
	cc       codec.Codec //编解码器
	opt      *Option
	sending  sync.Mutex // protect following
	header   codec.Header
	mu       sync.Mutex // protect following
	seq      uint64
	pending  map[uint64]*Call
	closing  bool // user has called Close
	shutdown bool // r has told us to stop
}

//显示声明，确保client实现了连接的关闭
var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

func (call *Call) done() {
	call.Done <- call
}

// Dial 发起连接，network, address是服务端地址
// 当拿到连接后，便可以在连接上构建客户端

type NewClientResult struct {
	client *Client
	err    error
}

func Dial(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewClient, network, address, opts...)
}

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	//DialTimeout处理
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan NewClientResult)
	if opt.ConnectTimeout == 0 {
		return NewClient(conn, opt)
	}
	go func() {
		//这里开启了一个goroutine,在最后写进了ch中，如果超时了
		//res := <-ch:就不会被执行了，也就是携程会一直阻塞
		client, err := f(conn, opt)
		ch <- NewClientResult{client: client, err: err}
	}()
	idleDelay := time.NewTimer(opt.ConnectTimeout)
	defer idleDelay.Stop()
	select {
	case res := <-ch:
		return res.client, res.err
	case <-idleDelay.C:
		go func() {
			<-ch
		}()
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	}

	return nil, err
}

//当没有参数或者是一个空的参数时，返回默认的option
//当传入一个参数时，把这个参数解析成option
//传入多个参数是错误的，不支持的
func parseOptions(opts ...*Option) (*Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	//根据option中的CodecType选择合适的序列化工具构建编解码器
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with r
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		return nil, err
	}
	client := &Client{
		seq:     1, // seq starts with 1, 0 means invalid call
		cc:      f(conn),
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	//客户端在创建立即接受服务端的消息，避免消息的丢失
	go client.receive()
	return client, nil
}

//sending锁是用来控制对pending的访问使用的
func (client *Client) send(call *Call) {
	// 发送前需要获得锁资源
	client.sending.Lock()
	//最后释放
	defer client.sending.Unlock()

	// 在发送前记录这个call的状态
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	//prepare request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	//header := new(codec.Header)
	//header.ServiceMethod = call.ServiceMethod
	//header.Seq = seq
	//header.Error = ""

	// encode and send the request
	//将请求发出
	if err := client.cc.Encode(&client.header, call.Args); err != nil {
		//因为事先标记了call为正在处理
		//此时没办法发出的话，要修改call的状态
		call := client.removeCall(seq)
		// 删除的时候如果为空,则代表call在其他处被处理了
		// 如果不为空，记录错误信息
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			// it usually means that Write partially failed
			// and call was already removed.
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// error occurs, so terminateCalls pending calls
	client.terminateCalls(err)
}

//从pending中删除一个call
//因为pending中存的是正在处理的call,但客户到收到应答时，说明已经处理完了。
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

//显示声明，确保client实现了连接的关闭

// Close 关闭连接，实际上是调用编解码里的Close()
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	// 如果连接已经关闭，这报错
	if client.closing {
		return ErrShutdown
	}
	//将closing置为true，随后关闭连接
	client.closing = true
	return client.cc.Close()
}

// IsAvailable 判断客户端当前是否可用
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	//closing和shutdown只要有一个为真，client便是不可用状态
	return !client.shutdown && !client.closing
}

//Go 和 Call 是客户端暴露给用户的两个 RPC 服务调用接口，Go 是一个异步接口，返回 call 实例。
//Call 是对 Go 的封装，阻塞 call.Done，等待响应返回，是一个同步接口。
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete,
// and returns its error status.
func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {

	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))

	select {
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}
