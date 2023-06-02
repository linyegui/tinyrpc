package registry

import (
	"context"
	"fmt"
	"go.etcd.io/etcd/clientv3"
	"time"
)

const (
	defaultEndpoint    = "localhost:2379"
	defaultDialTimeout = 5 * time.Second
)

func RegistryETCD(addr string) {
	// 创建ETCD客户端
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"}, // ETCD节点的地址
		DialTimeout: 5 * time.Second,            // 连接超时时间
	})
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	// 创建一个租约
	leaseResp, err := cli.Grant(context.Background(), 10)
	if err != nil {
		panic(err)
	}

	// 将服务注册到ETCD中
	key := "/services/" + addr
	value := "tcp@" + addr
	_, err = cli.Put(context.Background(), key, value, clientv3.WithLease(leaseResp.ID))
	if err != nil {
		panic(err)
	}

	// 续租
	keepAlive, err := cli.KeepAlive(context.Background(), leaseResp.ID)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			<-keepAlive
		}
	}()

	fmt.Println("Service registered successfully!")
	select {}
}
