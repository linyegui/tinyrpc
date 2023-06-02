package xclient

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"go.etcd.io/etcd/clientv3"
	"log"
	"time"
)

type EtcdDiscovery struct {
	//RegistryDiscovery 嵌套了 MultiServersDiscovery，很多能力可以复用
	*MultiServersDiscovery
	c   *clientv3.Client
	key string
}

const (
	defaultEndpoint    = "localhost:2379"
	defaultDialTimeout = 5 * time.Second
	defaultKey         = "/services"
)

func NewETCD(endpoints []string, duration time.Duration) (d *EtcdDiscovery, err error) {
	var c *clientv3.Client
	var conf clientv3.Config
	if len(endpoints) == 0 {
		conf.Endpoints = []string{defaultEndpoint}
	}
	if duration == 0 {
		conf.DialTimeout = defaultDialTimeout
	}
	c, err = clientv3.New(conf) // 连接超时时间
	if err != nil {
		panic(err)
	}

	var e = EtcdDiscovery{
		MultiServersDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		c:                     c,
	}
	d = &e
	return
}

func (d *EtcdDiscovery) Refresh() error {
	watchChan := d.c.Watch(context.Background(), d.key, clientv3.WithPrefix())
	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			switch event.Type {
			case mvccpb.PUT:
				d.AddServer(string(event.Kv.Value))
			case mvccpb.DELETE:
				d.DeleteServer(string(event.Kv.Value))
			}
		}
	}
	return nil
}

func (d *EtcdDiscovery) DeleteServer(server string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := 0; i < len(d.servers); i++ {
		if d.servers[i] == server {
			d.servers = append(d.servers[:i], d.servers[i+1:]...)
			return
		}
	}
}

func (d *EtcdDiscovery) AddServer(server string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = append(d.servers, server)
}

func (d *EtcdDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

func (d *EtcdDiscovery) GetAll() ([]string, error) {
	return d.MultiServersDiscovery.GetAll()
}

func (d *EtcdDiscovery) Get(mode SelectMode) (string, error) {
	return d.MultiServersDiscovery.Get(mode)
}

func LoadAndFresh(e *EtcdDiscovery) {
	key := "/services"
	resp, err := e.c.Get(context.Background(), key, clientv3.WithPrefix())
	if err != nil {
		panic(err)

	}
	server := make([]string, 0)
	for _, kv := range resp.Kvs {
		server = append(server, string(kv.Value))
	}
	err = e.Update(server)
	if err != nil {
		log.Panicln("更新本地服务列表失败:" + err.Error())
		return
	}
	if len(server) == 0 {
		log.Printf("没有发现服务")
	} else {
		fmt.Println(server)
	}
	fmt.Println(e.servers)

}
