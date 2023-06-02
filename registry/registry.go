package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// GeeRegistry 服务注册中心
type GeeRegistry struct {
	timeout time.Duration
	mu      sync.Mutex             // protect following
	servers map[string]*ServerItem //这里保留着所有可用的服务器的信息
}

// ServerItem 服务器信息
type ServerItem struct {
	Addr  string    //服务器地址
	start time.Time //用于判断服务器是否存货
}

const (
	//注册中心的地址
	defaultPath    = "/_tinyrpc_/registry"
	defaultTimeout = time.Minute * 5
)

// New 创建一个注册中心
func New(timeout time.Duration) *GeeRegistry {
	return &GeeRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultGeeRegister = New(defaultTimeout)

//往注册中心中添加服务，传入的是服务器地址
func (r *GeeRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		s.start = time.Now() // if exists, update start time to keep alive
	}
}

//返回一个切片，内部包含所有存活服务器的信息
func (r *GeeRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

// 处理Http请求
func (r *GeeRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// keep it simple, server is in req.Header
		w.Header().Set("X-tinyrpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		// keep it simple, server is in req.Header
		addr := req.Header.Get("X-tinyrpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleHTTP registers an HTTP handler for GeeRegistry messages on registryPath
func (r *GeeRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultGeeRegister.HandleHTTP(defaultPath)
}

// Heartbeat send a heartbeat message every once in a while
// it's a helper function for a server to register or send heartbeat
func Heartbeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		// make sure there is enough time to send heart beat
		// before it's removed from registry
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

//服务器放松心跳包给注册中心，怎么自己还存活
func sendHeartbeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	//创建一个http客户端
	httpClient := &http.Client{}
	//创建一个http post 请求，请求地址为注册中心
	req, _ := http.NewRequest("POST", registry, nil)
	//将服务器地址存在请求头
	req.Header.Set("X-tinyrpc-Server", addr)
	//发送
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
