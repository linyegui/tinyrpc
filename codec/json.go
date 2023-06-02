package codec

//gob为Go自带的序列化工具包

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

// GobCodec 定义了一种Gob类型的解码器
type JsonCodec struct {
	//解码器读写的对象，一个连接
	conn io.ReadWriteCloser
	//写缓冲区
	buf *bufio.Writer
	dec *json.Decoder
	enc *json.Encoder
}

//用于提醒是否GobCodec是否实现了Codec
//需要先实现Codec的接口，才能实现NewCodecFunc
var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	//创建一个带缓冲的写入器对象buf，将conn作为写入的目标
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf:  buf,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(buf), //编码器并不会直接写入Conn,而是先写进缓冲buf，在由buf写进连接
	}
}

func (c *JsonCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *JsonCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *JsonCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}

func (c *JsonCodec) Close() error {
	return c.conn.Close()
}

func (c *JsonCodec) Decode(h *Header, body interface{}) (err error) {
	err = c.dec.Decode(h)
	err = c.dec.Decode(body)
	return
}
func (c *JsonCodec) Encode(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}
