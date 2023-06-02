package xclient

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// RegistryDiscovery 服务发现
type RegistryDiscovery struct {
	//RegistryDiscovery 嵌套了 MultiServersDiscovery，很多能力可以复用
	*MultiServersDiscovery
	//注册中心的地址
	registry string
	//服务列表的过期时间，超过这个数就过期
	timeout time.Duration
	//最后一次更新的时间
	lastUpdate time.Time
}

const defaultUpdateTimeout = time.Second * 10

func (d *RegistryDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	fmt.Println(servers)
	d.lastUpdate = time.Now()
	return nil
}

func (d *RegistryDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", d.registry)
	resp, err := http.Get(d.registry)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	servers := strings.Split(resp.Header.Get("X-tinyrpc-Servers"), ",")
	d.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		//取出两端空字符
		if strings.TrimSpace(server) != "" {
			d.servers = append(d.servers, strings.TrimSpace(server))
		}
	}
	d.lastUpdate = time.Now()
	return nil
}

// Get 每次请求前都刷新？
func (d *RegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil {
		return "", err
	}
	return d.MultiServersDiscovery.Get(mode)
}

func (d *RegistryDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServersDiscovery.GetAll()
}

// NewRegistryDiscovery 创建服务发现
func NewRegistryDiscovery(registerAddr string, timeout time.Duration) *RegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	d := &RegistryDiscovery{
		MultiServersDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:              registerAddr,
		timeout:               timeout,
	}
	return d
}
