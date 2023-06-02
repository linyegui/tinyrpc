package codec

import "io"

//序列化和反序列化是将数据结构转换为字节流或将字节流转换为数据结构的过程
//该包负责从基于字节流的TCP连接上解析出双方想要的数据

// Header RPC请求头
type Header struct {
	// 客户端需要远程调用的方法 格式为:"Service.Method"
	//其中Service为服务名，Method为该下服务的方法名
	ServiceMethod string
	Seq           uint64 //用于标识当前RPC请求的唯一标识符
	Error         string //错误信息
}

// Codec Codec代表编解码器(Coder-Decoder)，是一个接口类型
// 它定义了从连接上读写数据的方法，和关闭连接的方法
type Codec interface {
	io.Closer                          //它定义了一个Close()方法，用于关闭连接资源
	ReadHeader(*Header) error          //用于从连接中读取请求头
	ReadBody(interface{}) error        //用于从连接中读取请求体
	Write(*Header, interface{}) error  // 将请求头和请求体写进连接中
	Encode(*Header, interface{}) error // 功能与Write类似
}

type NewCodec interface {
	io.Closer
	Encode(*Header, interface{}) error
	Decode(*Header, interface{}) error
}

// NewCodecFunc Go语言中，函数也是一种类型
// 这里规定了创建方法编解码器的函数必须是以io.ReadWriteCloser，以Codec为返回值
type NewCodecFunc func(io.ReadWriteCloser) Codec

// Type 表示编解码器的类型
// 通常RPC支持多种序列化和反序列化方式，
// 客户端和服务端需要靠Type来确定双方使用的是同一种序列化方式
type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" // not implemented
)

// NewCodecFuncMap 根据Type找到对应该编解码器类型的构造函数
var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	NewCodecFuncMap[JsonType] = NewJsonCodec
}
