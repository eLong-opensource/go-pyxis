package pyxis

import (
	"fmt"
	"log"
	"time"

	"github.com/eLong-INF/go-pyxis/watch"
)

func ExampleClient() {
	//pyixs服务端地址列表
	var addrs = []string{"127.0.0.1:7001", "127.0.0.1:7002", "127.0.0.1:7003"}
	w := &Watcher{
		Recursive: true,
		//是否监听子孙节点的事件
		Done: make(chan *watch.Event, 1),
	}
	var opt = &Options{
		DefaultWatcher: w,
		SessionTimeout: 5 * time.Second,
	}
	cli := NewClient(addrs, opt)
	defer cli.Close()
	//第一个参数设置创建的节点的类型,创建test节点并写入数据
	err := cli.Create(nil, "/test", []byte("createTest"))
	if err != nil {
		log.Fatalf("create path error:%s", err)
	}

	//读取/test节点，读取Data域
	n, err := cli.Read(nil, "/test", w)
	if err != nil {
		log.Fatalf("read path error:%s", err)
	}
	fmt.Println(n.Data)
	//Output("createTest")

	//更新/test节点的数据域
	err = cli.Write(nil, "/test", []byte("writeTest"))
	if err != nil {
		log.Fatalf("write path error:%s", err)
	}

	//删除test节点
	err = cli.Delete(nil, "/test")
	if err != nil {
		log.Fatalf("delete path error:%s", err)
	}
}
