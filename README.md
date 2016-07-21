# pyxis
--
    import "github.com/eLong-INF/go-pyxis"


## Usage

```go
var (
	ErrClosed          = errors.New("use of closed client")
	ErrConnecting      = errors.New("client is connecting")
	ErrNotFound        = errors.New("node not found")
	ErrNotLeader       = errors.New("not leader")
	ErrSessionExpired  = errors.New("session expired")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrAgain           = errors.New("try again")
	ErrExists          = errors.New("node exists")
	ErrInternal        = errors.New("internal error")
	ErrUnknown         = errors.New("unknown error")
)
```

```go
var Log = log15.New()
```
Log 设置日志输出对象

#### type Client

```go
type Client struct {
	DefaultWatcher *Watcher
}
```

Client 代表了一个资源定位的客户端，Client会自动从服务端列表里面找到Leader进行连接 Client连接成功后会定时发心跳 网络出问题后会自动重连

#### func  NewClient

```go
func NewClient(addrs []string, opt *Options) *Client
```
NewClient 使用一个服务端的地址列表和配置构造一个客户端对象 Client会自动从addrs里面获取到Leader进行连接

#### func (*Client) Close

```go
func (c *Client) Close() (err error)
```
Close 关闭连接，释放资源，同时删除会话，意味着所有的临时节点以及Watcher全部失效

#### func (*Client) Create

```go
func (c *Client) Create(opt *WriteOption, path string, data []byte) error
```
Create 创建一个节点，同时有一个可选的初始化数据 如果创建的是临时节点会跟当前的会话挂钩。 Create不会递归创建节点。 opt如果为空则使用默认值

#### func (*Client) Delete

```go
func (c *Client) Delete(opt *WriteOption, path string) error
```
Delete 删除给定的节点 opt为空则使用默认值

#### func (*Client) Read

```go
func (c *Client) Read(opt *ReadOption, path string, w *Watcher) (*node.Node, error)
```
Read 返回给定节点的数据及属性，同时有一个可选的Watcher来监听节点的变化 opt为空则使用默认值

#### func (*Client) Sid

```go
func (c *Client) Sid() (uint64, error)
```
Sid 返回当前客户端的session id

#### func (*Client) Unregister

```go
func (c *Client) Unregister() error
```
Unregister 注销当前会话但不关闭连接

#### func (*Client) Write

```go
func (c *Client) Write(opt *WriteOption, path string, data []byte) error
```
Write 向给定节点写入数据 opt为空则使用默认值

#### type Options

```go
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
```

Options 定义了Client的连接参数

#### type ReadOption

```go
type ReadOption struct {
	// 是否显示隐藏子节点
	ShowHidden bool
}
```

ReadOption 为读取操作的一些配置

#### type Watcher

```go
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
```

Watcher 代表了对某个节点的监听

#### type WriteOption

```go
type WriteOption struct {
	// for Create
	IsEphemeral bool
}
```

WriteOption 为写入操作的一些配置
