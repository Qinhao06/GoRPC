package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	services map[string]*ServerItem
	timeOut  time.Duration
	mu       *sync.Mutex
}

type ServerItem struct {
	Addr       string
	UpdateTime time.Time
}

const (
	defaultPath    = "/_rpc_/registry"
	defaultTimeout = time.Minute * 5
)

func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*ServerItem),
		timeOut:  defaultTimeout,
		mu:       new(sync.Mutex),
	}
}

var DefaultRegistry = NewRegistry()

func (r *Registry) AddServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.services[addr]
	if item == nil {
		item = &ServerItem{
			Addr:       addr,
			UpdateTime: time.Now(),
		}
		r.services[addr] = item
	} else {
		item.UpdateTime = time.Now()
	}
}

func (r *Registry) RemoveServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.services, addr)
}

func (r *Registry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, item := range r.services {
		if time.Now().Sub(item.UpdateTime) < r.timeOut {
			alive = append(alive, addr)
		} else {
			delete(r.services, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// keep it simple, server is in req.Header
		w.Header().Set("X-Rpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		// keep it simple, server is in req.Header
		addr := req.Header.Get("X-Rpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.AddServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *Registry) HandleHttp() {
	http.Handle(defaultPath, r)
	log.Println("registry listen on", defaultPath)
}

func HandleHttp() {
	DefaultRegistry.HandleHttp()
}

func HeartBeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Duration(time.Minute)
	}
	var err error
	err = SendHeartBeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = SendHeartBeat(registry, addr)
		}
	}()
}

func SendHeartBeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Rpc-Server", addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
