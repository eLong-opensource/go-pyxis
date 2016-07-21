package pyxis

import (
	"errors"
	"math/rand"
	"net/rpc"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/eLong-INF/go-pyxis/node"
	prpc "github.com/eLong-INF/go-pyxis/rpc"
	"github.com/eLong-INF/go-pyxis/store"
	"github.com/eLong-INF/go-pyxis/watch"

	"gopkg.in/inconshreveable/log15.v2"
)

const (
	defaultSessionTimeout = 5 * time.Second
)

// state代表了Client的状态，内部使用
type state int32

const (
	connecting state = iota
	connected
	closed
)

// Log 设置日志输出对象
var Log = log15.New()

// Client 代表了一个资源定位的客户端，Client会自动从服务端列表里面找到Leader进行连接
// Client连接成功后会定时发心跳
// 网络出问题后会自动重连
type Client struct {
	addrs          []string
	DefaultWatcher *Watcher
	timeout        time.Duration

	watcherlock sync.Mutex
	watchers    map[string]*Watcher

	log log15.Logger

	// 当前连接的地址，日志需要
	remote string

	mutex sync.Mutex // protecting follows
	state state
	sid   uint64
	conn  *rpc.Client
}

// Options 定义了Client的连接参数
type Options struct {
	// DefaultWatcher为客户端的默认Watcher，当一个事件到达的时候如果没有可用的Watcher就
	// 会使用这个来通知事件。同时也会接收超时，连接和断开事件。
	DefaultWatcher *Watcher

	// SessionTimeout设置会话的过期时间，这个时间不能超过服务端设置的最大和最小超时时间
	// 默认为5秒
	SessionTimeout time.Duration

	// 如果Sid不等于0，则设置了默认的连接会话id，这个sid代表的会话可能已经过期
	Sid uint64
}

// NewClient 使用一个服务端的地址列表和配置构造一个客户端对象
// Client会自动从addrs里面获取到Leader进行连接
func NewClient(addrs []string, opt *Options) *Client {
	if addrs == nil {
		return nil
	}
	cli := &Client{
		addrs:          addrs,
		DefaultWatcher: opt.DefaultWatcher,
		watchers:       make(map[string]*Watcher),
		state:          connecting,
		timeout:        opt.SessionTimeout,
		sid:            opt.Sid,
	}
	sidfunc := func() uint64 { return cli.sid }
	addrfunc := func() string { return cli.remote }
	cli.log = Log.New("sid", log15.Lazy{Fn: sidfunc}, "addr", log15.Lazy{Fn: addrfunc})

	if opt.SessionTimeout < defaultSessionTimeout {
		cli.timeout = defaultSessionTimeout
	}
	rand.Seed(time.Now().Unix())
	go cli.register(cli.sid)
	return cli
}

func randAddr(addrs []string) string {
	return addrs[rand.Intn(len(addrs))]
}

func (c *Client) register(sid uint64) {
	var remote string
	var conn *rpc.Client
	var err error

	var req *prpc.Request
	var rep *prpc.Response

	// sleep interval
	interval := 10 * time.Millisecond
	remote = randAddr(c.addrs)
	for c.state != closed {
		c.log.Info("rpc.register")
		c.log.Info("rpc.connect", "addr", remote)
		conn, err = prpc.Dial(remote)
		if err != nil {
			c.log.Error("rpc.call", "error", err)
			remote = randAddr(c.addrs)
			goto retry
		}
		c.log.Info("rpc.connected")
		req = new(prpc.Request)
		rep = new(prpc.Response)
		req.Register = &prpc.RegisterRequest{}
		req.Register.Timeout = proto.Int32(int32(c.timeout / time.Millisecond))
		if sid != 0 {
			c.log.Info("usd sid to register", "sid", sid)
			req.Sid = &sid
		}
		err = conn.Call("pyxis.rpc.RpcService.Call", req, rep)
		if err != nil {
			c.log.Error("rpc.register failed", "error", err)
			remote = randAddr(c.addrs)
			goto retry
		}

		// no error, use this connection
		if rep.Err == nil {
			sid = rep.GetSid()
			c.log.Info("rpc.Register success")
			go c.pingLoop(sid, remote, conn)
			return
		}

		if rep.Err != nil {
			c.log.Error("rpc.register error", "type", rep.GetErr().GetType(),
				"msg", string(rep.GetErr().GetData()))
		}

		switch rep.GetErr().GetType() {
		case prpc.ErrorType_kNotLeader:
			c.log.Error("rpc.register: not leader")
			s := rep.GetErr().GetData()
			if s != nil {
				// current not leader, use leader address
				remote = string(s)
				interval = 100 * time.Millisecond
			}
		case prpc.ErrorType_kSessionExpired:
			c.log.Error("rpc.register session expired, reset sid")
			sid = 0
			// 继续使用当前的连接
			interval = 100 * time.Millisecond
		default:
			remote = randAddr(c.addrs)
		}

	retry:
		if conn != nil {
			conn.Close()
		}
		time.Sleep(interval)
		interval = interval * 2
		if interval > 30*time.Second {
			interval = 30 * time.Second
		}
	}
}

func (c *Client) pingLoop(sid uint64, remote string, conn *rpc.Client) {
	c.remote = remote

	c.log.Info("pingloop", "newsid", sid, "oldsid", c.sid)
	expired := (c.sid != 0 && c.sid != sid)

	c.mutex.Lock()
	c.state = connected
	c.sid = sid
	c.conn = conn
	c.mutex.Unlock()

	c.triggerDefaultEvent(watch.Connected)

	if expired {
		c.log.Info("trigger expired event")
		c.triggerDefaultEvent(watch.SessionExpired)
	}

	// 重新注册之前的watcher
	go c.reregisterWatchers()

	var err error
	req := prpc.Request{}
	rep := prpc.Response{}
	req.Ping = &prpc.PingRequest{}
	for c.state != closed {
		c.log.Debug("rpc.Ping")
		err = c.call(&req, &rep)
		if err != nil {
			c.log.Error("rpc.ping error", "error", err)
			break
		}
		if rep.Watch != nil {
			w := watch.Event{
				Path: rep.Watch.GetPath(),
				Type: watch.EventType(rep.Watch.GetType()),
			}

			c.log.Info("get event", "path", w.Path, "event", w.Type)
			p := rep.GetPath()
			var watcher *Watcher
			var ok bool
			c.watcherlock.Lock()
			watcher, ok = c.watchers[p]
			if !ok {
				watcher = c.DefaultWatcher
			} else {
				delete(c.watchers, p)
			}
			c.watcherlock.Unlock()
			if watcher == nil {
				c.log.Info("no watcher, drop event", "event", w)
				continue
			}
			w.User = watcher.User
			err := c.triggerEvent(watcher.Done, &w)
			if err != nil {
				c.log.Error("send event", "event", w, "error", err)
			}
		}
	}
	c.log.Info("disconnected")
	c.state = connecting
	c.triggerDefaultEvent(watch.Disconnected)
	conn.Close()
	if c.state != closed {
		c.register(sid)
	}
}

func (c *Client) triggerDefaultEvent(tp watch.EventType) {
	if c.DefaultWatcher != nil {
		e := &watch.Event{
			Path: "",
			Type: tp,
			User: c.DefaultWatcher.User,
		}
		err := c.triggerEvent(c.DefaultWatcher.Done, e)
		if err != nil {
			c.log.Error("send event", "watcher", "defaultWatcher", "event", e, "error", err)
		}
	}
}

func (c *Client) triggerEvent(ch chan *watch.Event, event *watch.Event) error {
	var err error
	select {
	case ch <- event:
	default:
		err = errors.New("watch channel block")
	}
	return err
}

func (c *Client) reregisterWatchers() {
	rep := prpc.Response{}
	c.watcherlock.Lock()
	ch := make(chan *call, len(c.watchers))
	for p, w := range c.watchers {
		req := prpc.Request{}
		req.Watch = &prpc.WatchRequest{}
		req.Watch.Path = &p
		req.Watch.Watch = proto.Uint32(uint32(w.Filter))
		req.Watch.Recursive = proto.Bool(w.Recursive)
		c.goo(&req, &rep, ch)
	}
	c.watcherlock.Unlock()
}

func (c *Client) addWatcher(path string, w *Watcher) {
	c.watcherlock.Lock()
	c.watchers[path] = w
	c.watcherlock.Unlock()
}

type call struct {
	Req   *prpc.Request
	Rep   *prpc.Response
	Error error
	Done  chan *call
}

// Sid 返回当前客户端的session id
func (c *Client) Sid() (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	switch c.state {
	case connected:
		return c.sid, nil
	case connecting:
		return 0, ErrConnecting
	case closed:
		fallthrough
	default:
		return 0, ErrClosed
	}
}

func (c *Client) goo(req *prpc.Request, rep *prpc.Response, ch chan *call) *call {
	if ch == nil {
		ch = make(chan *call, 1)
	}
	call := &call{
		Req:  req,
		Rep:  rep,
		Done: ch,
	}

	var err error
	// fast path
	switch c.state {
	case connecting:
		err = ErrConnecting
	case closed:
		err = ErrClosed

	}
	if err != nil {
		call.Error = err
		call.Done <- call
		return call
	}

	c.mutex.Lock()
	req.Sid = proto.Uint64(c.sid)
	done := c.conn.Go("pyxis.rpc.RpcService.Call", req, rep, nil)
	c.mutex.Unlock()
	go func() {
		<-done.Done
		call.Error = done.Error
		call.Done <- call
	}()
	return call
}

func (c *Client) call(req *prpc.Request, rep *prpc.Response) error {
	done := c.goo(req, rep, nil)
	<-done.Done
	if rep.Err != nil {
		return newRPCError(rep.Err)
	}
	return done.Error
}

// Close 关闭连接，释放资源，同时删除会话，意味着所有的临时节点以及Watcher全部失效
func (c *Client) Close() (err error) {
	c.Unregister()
	c.mutex.Lock()
	if c.state == connected {
		err = c.conn.Close()
	}
	c.state = closed
	c.mutex.Unlock()
	return
}

// Create 创建一个节点，同时有一个可选的初始化数据
// 如果创建的是临时节点会跟当前的会话挂钩。
// Create不会递归创建节点。
// opt如果为空则使用默认值
func (c *Client) Create(opt *WriteOption, path string, data []byte) error {
	flag := node.Persist
	if opt != nil && opt.IsEphemeral {
		flag = node.Ephemeral
	}
	req := prpc.Request{}
	rep := prpc.Response{}
	req.Create = &prpc.CreateRequest{}
	req.Create.Path = &path
	req.Create.Flags = proto.Int32(int32(flag))
	if data != nil {
		req.Create.Data = data
	}
	return c.call(&req, &rep)
}

// Delete 删除给定的节点
// opt为空则使用默认值
func (c *Client) Delete(opt *WriteOption, path string) error {
	req := prpc.Request{}
	rep := prpc.Response{}
	req.Delete = &prpc.DeleteRequest{}
	req.Delete.Path = &path
	return c.call(&req, &rep)
}

// Watcher 代表了对某个节点的监听
type Watcher struct {
	// Recursive如果为true代表递归监听这个节点及子节点的事件
	Recursive bool
	// Filter为事件的集合，代表了只监听这些事件
	Filter watch.EventType
	// Done为接收事件的channel，客户端不会阻塞等待发送数据，意味着如果channel
	// 没有在接收状态，这个消息就丢了
	Done chan *watch.Event
	// User为用户自定义的数据，在watch.Event里面会附带上
	User interface{}
}

// Read 返回给定节点的数据及属性，同时有一个可选的Watcher来监听节点的变化
// opt为空则使用默认值
func (c *Client) Read(opt *ReadOption, path string, w *Watcher) (*node.Node, error) {
	req := prpc.Request{}
	rep := prpc.Response{}

	req.Stat = &prpc.StatRequest{}
	req.Stat.Path = &path
	if w != nil {
		c.addWatcher(path, w)
		req.Watch = &prpc.WatchRequest{}
		req.Watch.Path = &path
		req.Watch.Watch = proto.Uint32(uint32(w.Filter))
		req.Watch.Recursive = proto.Bool(w.Recursive)
	}
	err := c.call(&req, &rep)
	if err != nil {
		return nil, err
	}
	protoNode := new(store.Node)
	err = proto.Unmarshal(rep.GetData(), protoNode)
	if err != nil {
		return nil, err
	}

	// process children
	var children []string
	if opt != nil && opt.ShowHidden {
		children = protoNode.GetChildren()
	} else {
		var c1 []string
		for _, c := range protoNode.GetChildren() {
			if len(c) > 0 && c[0] == '_' {
				continue
			}
			c1 = append(c1, c)
		}
		children = c1
	}
	return &node.Node{
		Data:     protoNode.GetData(),
		Children: children,
		Flags:    node.Flag(protoNode.GetFlags()),
		Version:  protoNode.GetVersion(),
		Owner:    protoNode.GetSid(),
	}, nil
}

// Write 向给定节点写入数据
// opt为空则使用默认值
func (c *Client) Write(opt *WriteOption, path string, data []byte) error {
	req := prpc.Request{}
	rep := prpc.Response{}
	req.Write = &prpc.WriteRequest{}
	req.Write.Path = &path
	req.Write.Data = data
	return c.call(&req, &rep)
}

// Unregister 注销当前会话但不关闭连接
func (c *Client) Unregister() error {
	req := prpc.Request{}
	rep := prpc.Response{}
	req.Unregister = &prpc.UnRegisterRequest{}
	return c.call(&req, &rep)
}

func init() {
	Log.SetHandler(log15.DiscardHandler())
}
