package socks

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/armon/go-socks5"
	"github.com/bytedance/gopkg/lang/mcache"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/taodev/goway/config"
	"github.com/taodev/goway/internal/geoip"
	"github.com/taodev/goway/internal/http"
	"github.com/taodev/goway/internal/myssh"
	"github.com/taodev/goway/internal/netflow"
	"github.com/taodev/goway/internal/socks"
)

type SocksV5Server struct {
	netflow.Netflow

	Options   config.NodeConfig
	Listener  net.Listener
	sshDialer *myssh.SSHClient
	socks     *socks5.Server
}

func (svr *SocksV5Server) ConnectRemoteSSH() (err error) {
	opts := svr.Options
	if err = geoip.Load(); err != nil {
		return
	}

	if svr.sshDialer, err = myssh.NewSSHClient(opts.SSH.URL, opts.SSH.IdentityFile); err != nil {
		return
	}

	return
}

func (svr *SocksV5Server) Listen() (err error) {
	if err = svr.ConnectRemoteSSH(); err != nil {
		return
	}

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Printf("network: %s socks: %s", network, addr)
			return svr.sshDialer.Dial(network, addr)
		},
	}

	if svr.socks, err = socks5.New(conf); err != nil {
		return
	}

	if err = svr.socks.ListenAndServe("tcp", svr.Options.Addr); err != nil {
		log.Fatal(err)
	}

	return
}

func (svr *SocksV5Server) ListenTCP() (err error) {
	if err = svr.ConnectRemoteSSH(); err != nil {
		return
	}

	svr.Listener, err = net.Listen("tcp", svr.Options.Addr)
	if err != nil {
		return
	}

	svr.Netflow.Start(60, func(i netflow.NetflowInfo) {
		log.Printf("netflow: conn: %v\ttotal: r-%v w-%v\tspeed: r-%v w-%v",
			i.ConnTotal,
			netflow.BytesFormat(i.ReadTotal), netflow.BytesFormat(i.WrittenTotal),
			netflow.BytesFormat(i.ReadSpeed), netflow.BytesFormat(i.WrittenSpeed),
		)

		// 打印系统内存使用情况
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Printf("mem: alloc-%v sys-%v heap-%v stack-%v",
			netflow.BytesFormat(int64(m.Alloc)), netflow.BytesFormat(int64(m.Sys)),
			netflow.BytesFormat(int64(m.HeapAlloc)), netflow.BytesFormat(int64(m.StackInuse)),
		)
	})

	go func() {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("ListenTCP crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
			}
		}()

		for {
			var conn net.Conn
			conn, err = svr.Listener.Accept()
			if err != nil {
				log.Printf("accept error , ERR:%s", err)
				break
			}
			conn = &myssh.SSHConn{
				Conn: conn,
			}

			gopool.Go(func() {
				defer func() {
					if e := recover(); e != nil {
						log.Printf("connection handler crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
					}
				}()

				svr.executeConn(conn)
			})
		}
	}()

	log.Printf("http http(s) proxy on %s", svr.Options.Addr)
	return
}

// socks5 handshake
func (svr *SocksV5Server) executeConn(conn net.Conn) {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("executeConn crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
		}
	}()

	defer conn.Close()

	// socks5 handshake
	if err := socks.Socks5Handshake(conn); err != nil {
		log.Printf("socks5 handshake error , ERR:%s", err)
		return
	}

	// socks5 request
	req, err := socks.Socks5Request(conn)
	if err != nil {
		log.Printf("socks5 request error , ERR:%s", err)
		return
	}

	log.Println("req:", req)

	// socks5 response
	if err = socks.Socks5Response(conn, req); err != nil {
		log.Printf("socks5 response error , ERR:%s", err)
		return
	}

	// socks5 connect
	var address string
	switch req.Atyp {
	case 0x01:
		address = fmt.Sprintf("%s:%d", req.DstAddr, req.DstPort)
	case 0x03:
		address = fmt.Sprintf("%s:%d", req.Host, req.DstPort)
	case 0x04:
		address = fmt.Sprintf("[%s]:%d", req.DstAddr, req.DstPort)
	}

	if err = socks.Socks5Connect(conn, req); err != nil {
		log.Printf("socks5 connect error , ERR:%s", err)
		return
	}

	err = svr.OutToTCP(address, &conn, req)

	if err != nil {
		log.Printf("connect to %s fail, ERR:%s", address, err)
		http.CloseConn(&conn)
	}
}

func (svr *SocksV5Server) OutToTCP(address string, inConn *net.Conn, req *socks.Socks5RequestData) (err error) {
	inAddr := (*inConn).RemoteAddr().String()
	inLocalAddr := (*inConn).LocalAddr().String()

	log.Println("targetAddress:", address)

	var outConn net.Conn
	// 匹配跳板规则
	bridge, ok := svr.Options.Match(address)
	if ok {
		outConn, err = svr.sshDialer.Dial("tcp", bridge)
		log.Println("bridge:", bridge, "host:", address)
	} else {
		if geoip.InPRC(address) {
			outConn, err = net.Dial("tcp", address)
		} else {
			outConn, err = svr.sshDialer.Dial("tcp", address)
		}
	}

	if err != nil {
		log.Printf("connect to %s, err:%s", address, err)
		http.CloseConn(inConn)
		return
	}

	// socks5 connect
	if req.Cmd == 0x01 {
		// socks5 connect
		// _, err = (*inConn).Write([]byte{0x05, 0x00, 0x00, 0x01})
		req.Request[1] = 0x00
		_, err = (*inConn).Write(req.Request)
		if err != nil {
			return
		}

		// 二进制打印协议
		fmt.Printf("n: %x\n", req.Request)
	}

	svr.Netflow.AddConn(1)

	svr.IoBind((*inConn), outConn, func(err error) {
		log.Printf("conn %s - %s released [%s]", inAddr, inLocalAddr, req.Host)

		http.CloseConn(inConn)
		http.CloseConn(&outConn)
	})

	log.Printf("conn %s - %s connected [%s]", inAddr, inLocalAddr, req.Host)
	return
}

func (svr *SocksV5Server) IoBind(src, dst net.Conn, fnClose func(err error)) {
	var one = &sync.Once{}
	dst = &netflow.NetflowConn{
		Conn:    dst,
		Netflow: &svr.Netflow,
	}

	gopool.Go(func() {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("IoBind crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
			}
		}()

		buf := mcache.Malloc(32 * 1024)
		defer mcache.Free(buf)

		if _, err := io.CopyBuffer(dst, src, buf); err != nil {
			one.Do(func() {
				fnClose(err)
			})
		}

		svr.Netflow.DelConn(1)
	})

	gopool.Go(func() {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("IoBind crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
			}
		}()

		buf := mcache.Malloc(32 * 1024)
		defer mcache.Free(buf)

		if _, err := io.CopyBuffer(src, dst, buf); err != nil {
			one.Do(func() {
				fnClose(err)
			})
		}
	})
}

func (svr *SocksV5Server) Shutdown() {
	svr.Listener.Close()
	svr.Netflow.Stop()
	svr.sshDialer.Shutdown()
}

func (svr *SocksV5Server) Run() (err error) {
	if err = svr.Listen(); err != nil {
		log.Println("Listen:", err)
		return
	}

	return
}

func NewSocksV5Server(opts config.NodeConfig) (svr *SocksV5Server) {
	svr = new(SocksV5Server)
	svr.Options = opts
	return
}
