package geoip

import (
	"net"
	"sync"
)

// DNSCache
type DNSCache struct {
	dnsCache       map[string]net.IP
	dnsCacheLocker sync.Mutex
}

// 查询并保存DNS缓存
func (cache *DNSCache) Query(domain string) (ip net.IP, ok bool) {
	cache.dnsCacheLocker.Lock()
	defer cache.dnsCacheLocker.Unlock()

	ip, ok = cache.dnsCache[domain]
	if !ok {
		addr, err := net.ResolveIPAddr("ip", domain)
		if err != nil {
			return
		}

		ok = true
		ip = addr.IP
		cache.dnsCache[domain] = ip
	}

	// log.Printf("dns cache: %s -> %s", domain, ip.String())

	return
}

// 初始化DNS缓存
func NewDNSCache() *DNSCache {
	cache := new(DNSCache)
	cache.dnsCache = make(map[string]net.IP)
	return cache
}
