package client

import (
	"errors"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelectMode SelectMode = iota
	RoundRobinSelectMode
)

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

type ServersDiscovery struct {
	servers []string
	ran     *rand.Rand
	mu      *sync.Mutex
	index   int
}

func (s *ServersDiscovery) Refresh() error {
	return nil
}

func (s *ServersDiscovery) Update(servers []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = servers
	return nil
}

func (s *ServersDiscovery) Get(mode SelectMode) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	length := len(s.servers)
	if length == 0 {
		return "", errors.New("no available server")
	}
	switch mode {
	case RandomSelectMode:
		return s.servers[s.ran.Intn(length)], nil
	case RoundRobinSelectMode:
		s.index = (s.index + 1) % length
		return s.servers[s.index], nil
	default:
		return "", errors.New("invalid select mode")
	}
}

func (s *ServersDiscovery) GetAll() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	servers := make([]string, len(s.servers))
	copy(servers, s.servers)
	return servers, nil
}

func NewServersDiscovery(servers []string) *ServersDiscovery {
	return &ServersDiscovery{
		servers: servers,
		ran:     rand.New(rand.NewSource(time.Now().UnixNano())),
		mu:      &sync.Mutex{},
		index:   rand.Intn(math.MaxInt - 1),
	}
}

var _ Discovery = (*ServersDiscovery)(nil)

// registry discovery

type ServersDiscoveryRegistry struct {
	*ServersDiscovery
	registry       string
	timeOut        time.Duration
	lastUpdateTime time.Time
}

const defaultTimeOut = time.Second * 10

func NewServersDiscoveryRegistry(registry string, timeOut time.Duration) *ServersDiscoveryRegistry {
	if timeOut == 0 {
		timeOut = defaultTimeOut
	}
	return &ServersDiscoveryRegistry{
		ServersDiscovery: NewServersDiscovery([]string{}),
		registry:         registry,
		timeOut:          timeOut,
	}
}

var _ Discovery = (*ServersDiscoveryRegistry)(nil)

func (s *ServersDiscoveryRegistry) Update(severs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = severs
	s.lastUpdateTime = time.Now()
	return nil
}

func (s *ServersDiscoveryRegistry) Refresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.lastUpdateTime) < s.timeOut {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", s.registry)
	resp, err := http.Get(s.registry)
	if err != nil {
		log.Println("rpc get registry error", err)
		return err
	}
	severs := strings.Split(resp.Header.Get("X-Rpc-Servers"), ",")
	s.servers = make([]string, 0, len(severs))
	for _, server := range severs {
		if strings.TrimSpace(server) != "" {
			s.servers = append(s.servers, strings.TrimSpace(server))
		}
	}
	s.lastUpdateTime = time.Now()
	return nil
}

func (s *ServersDiscoveryRegistry) Get(mode SelectMode) (string, error) {
	if err := s.Refresh(); err != nil {
		return "", err
	}
	return s.ServersDiscovery.Get(mode)
}

func (s *ServersDiscoveryRegistry) GetAll() ([]string, error) {
	if err := s.Refresh(); err != nil {
		return nil, err
	}
	return s.ServersDiscovery.GetAll()
}
