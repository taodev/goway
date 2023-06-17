package myssh

import (
	"log"
	"math/rand"
	"net"
	"sync"
)

// ssh 连接池
type SSHClientPool struct {
	URL      string
	Key      string
	MaxConns int

	sc     []*SSHClient
	locker sync.RWMutex
}

func (pool *SSHClientPool) Dial(n, addr string) (c net.Conn, err error) {
	var sc *SSHClient

	pool.locker.RLock()
	if pool.MaxConns <= 0 {
		return nil, ErrNotValid
	}

	// 从连接数组中用随机数挑选一个连接
	sc = pool.sc[rand.Intn(pool.MaxConns)]
	pool.locker.RUnlock()

	return sc.Dial(n, addr)
}

func (pool *SSHClientPool) Shutdown() {
	pool.locker.Lock()
	defer pool.locker.Unlock()

	for i := 0; i < pool.MaxConns; i++ {
		pool.sc[i].Shutdown()
	}
}

func NewSSHClientPool(url, key string, maxConns int) (pool *SSHClientPool, err error) {
	pool = &SSHClientPool{
		URL:      url,
		Key:      key,
		MaxConns: maxConns,
		sc:       make([]*SSHClient, maxConns),
	}

	for i := 0; i < maxConns; i++ {
		pool.sc[i], err = NewSSHClient(url, key)
		if err != nil {
			log.Printf("[ERROR] NewSSHClientPool NewSSHClient failed, err: %s", err)
			return
		}
	}

	return
}
