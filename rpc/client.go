package rpc

import (
	"bufio"
	"encoding/binary"
	"hash/adler32"
	"io"
	"net"
	"net/rpc"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
)

var (
	RPC_MAGIC = [4]byte{'p', 'y', 'x', 'i'}
)

type Packet struct {
	TotalSize uint32
	Magic     [4]byte
	Payload   []byte
	Checksum  uint32
}

// 4bytes total size
// 4bytes magic
// payload
// 4bytes adler32 checksum
func EncodePacket(w io.Writer, payload []byte) error {
	totalsize := uint32(len(RPC_MAGIC) + len(payload) + 4)
	binary.Write(w, binary.BigEndian, totalsize)
	sum := adler32.New()
	ww := io.MultiWriter(w, sum)
	binary.Write(ww, binary.BigEndian, RPC_MAGIC)
	ww.Write(payload)
	checksum := sum.Sum32()
	return binary.Write(w, binary.BigEndian, checksum)
}

type clientCodec struct {
	rw      *bufio.ReadWriter
	conn    io.ReadWriteCloser
	payload *Response
}

func NewClientCodec(rwc io.ReadWriteCloser) rpc.ClientCodec {
	return &clientCodec{
		rw: bufio.NewReadWriter(
			bufio.NewReader(rwc),
			bufio.NewWriter(rwc),
		),
		conn:    rwc,
		payload: nil,
	}
}

func (c *clientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	req, ok := param.(*Request)
	if !ok {
		panic("must be Request packet")
	}
	req.Xid = &r.Seq
	payload, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	EncodePacket(c.rw, payload)
	return c.rw.Flush()
}

func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	var totalsize uint32
	err := binary.Read(c.rw, binary.BigEndian, &totalsize)
	if err != nil {
		return errors.Annotate(err, "read total size")
	}

	// at least len(magic) + len(checksum)
	if totalsize < 8 {
		return errors.Errorf("bad packet. header:%d", totalsize)
	}

	sum := adler32.New()
	rr := io.TeeReader(c.rw, sum)

	var magic [4]byte
	err = binary.Read(rr, binary.BigEndian, &magic)
	if err != nil {
		return errors.Annotate(err, "read magic")
	}
	if magic != RPC_MAGIC {
		return errors.Errorf("bad rpc magic:%v", magic)
	}

	payload := make([]byte, totalsize-8)
	_, err = io.ReadFull(rr, payload)
	if err != nil {
		return errors.Annotate(err, "read payload")
	}

	var checksum uint32
	err = binary.Read(c.rw, binary.BigEndian, &checksum)
	if err != nil {
		return errors.Annotate(err, "read checksum")
	}

	if checksum != sum.Sum32() {
		return errors.Errorf("checkSum error, %d(calc) %d(remote)", sum.Sum32(), checksum)
	}

	rep := Response{}
	err = proto.Unmarshal(payload, &rep)
	if err != nil {
		return errors.Annotate(err, "decode message")
	}
	r.Seq = rep.GetXid()
	c.payload = &rep
	return nil
}

func (c *clientCodec) ReadResponseBody(x interface{}) error {
	if x == nil {
		return nil
	}
	if c.payload == nil {
		return errors.New("payload nil")
	}
	rep, ok := x.(*Response)
	if !ok {
		return errors.New("param must be *Response")
	}
	*rep = *c.payload
	return nil
}

func (c *clientCodec) Close() error {
	return c.conn.Close()
}

func NewClient(conn io.ReadWriteCloser) *rpc.Client {
	return rpc.NewClientWithCodec(NewClientCodec(conn))
}

func Dial(addr string) (*rpc.Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), err
}
