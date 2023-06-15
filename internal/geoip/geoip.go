package geoip

import (
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

const DOWNLOAD_GEOIP2_URL = "https://raw.githubusercontent.com/Hackl0us/GeoIP2-CN/release/Country.mmdb"
const GEOIP2_PATH = "geoip.mmdb"

var geoipDB *geoip2.Reader
var geoipDBLocker sync.Mutex
var dnsCache *DNSCache

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func Load() (err error) {
	if !FileExists(GEOIP2_PATH) {
		cmd := exec.Command("wget", "-O", GEOIP2_PATH, DOWNLOAD_GEOIP2_URL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			log.Fatalf("下载文件时发生错误：%v", err)
		}
	}

	geoipDB, err = geoip2.Open(GEOIP2_PATH)
	if err != nil {
		return
	}

	dnsCache = NewDNSCache()

	return
}

func Update() (err error) {
	geoipDBLocker.Lock()
	defer geoipDBLocker.Unlock()

	if geoipDB != nil {
		geoipDB.Close()
		geoipDB = nil
	}

	return Load()
}

func Close() {
	if geoipDB != nil {
		geoipDB.Close()
		geoipDB = nil
	}
}

func InPRC(addr string) bool {
	var err error
	host := addr
	if strings.Contains(host, ":") {
		if host, _, err = net.SplitHostPort(host); err != nil {
			log.Print("SplitHostPort:", host, err)
			return true
		}
	}

	// 解析域名为IP地址
	ip, ok := dnsCache.Query(host)
	if !ok {
		return true
	}

	// 查询IP地址的归属地
	record, err := geoipDB.Country(ip)
	if err != nil {
		log.Printf("geoip search country error: %s", err)
		return true
	}

	// 判断归属地是否是中国
	if record.Country.IsoCode == "CN" {
		return true
	}

	return false
}
