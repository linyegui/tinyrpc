package codec

//gob为Go自带的序列化工具包

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

// GobCodec 定义了一种Gob类型的解码器
type GobCodec struct {
	//解码器读写的对象，一个连接
	conn io.ReadWriteCloser
	//写缓冲区
	buf *bufio.Writer
	dec *gob.Decoder
	enc *gob.Encoder
}

//用于提醒是否GobCodec是否实现了Codec
//需要先实现Codec的接口，才能实现NewCodecFunc
var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	//创建一个带缓冲的写入器对象buf，将conn作为写入的目标
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf), //编码器并不会直接写入Conn,而是先写进缓冲buf，在由buf写进连接
	}
}

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
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

func (c *GobCodec) Close() error {
	return c.conn.Close()
}

func (c *GobCodec) Decode(h *Header, body interface{}) (err error) {
	err = c.dec.Decode(h)
	err = c.dec.Decode(body)
	return
}
func (c *GobCodec) Encode(h *Header, body interface{}) (err error) {
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
