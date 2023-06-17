package http

import (
	"io"
	"log"
	"net"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/bytedance/gopkg/lang/mcache"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/taodev/goway/config"
	"github.com/taodev/goway/internal/geoip"
	"github.com/taodev/goway/internal/http"
	"github.com/taodev/goway/internal/myssh"
	"github.com/taodev/goway/internal/netflow"
)

type HttpServer struct {
	netflow.Netflow

	Options  config.NodeConfig
	Listener net.Listener
	sshPool  *myssh.SSHClientPool
}

func (svr *HttpServer) ConnectRemoteSSH() (err error) {
	opts := svr.Options
	if err = geoip.Load(); err != nil {
		return
	}

	if svr.sshPool, err = myssh.NewSSHClientPool(opts.SSH.URL, opts.SSH.IdentityFile, 10); err != nil {
		return
	}

	return
}

func (svr *HttpServer) ListenTCP() (err error) {
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

func (svr *HttpServer) executeConn(inConn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("http(s) conn handler crashed with err : %s \nstack: %s", err, string(debug.Stack()))
		}
	}()

	req, err := http.NewHTTPRequest(&inConn, 4096)
	if err != nil {
		if err != io.EOF {
			log.Printf("decoder error , form %s, ERR:%s", err, inConn.RemoteAddr())
		}
		http.CloseConn(&inConn)
		return
	}

	address := req.Host

	err = svr.OutToTCP(address, &inConn, &req)

	if err != nil {
		log.Printf("connect to %s fail, ERR:%s", address, err)
		http.CloseConn(&inConn)
	}
}

func (svr *HttpServer) OutToTCP(address string, inConn *net.Conn, req *http.HTTPRequest) (err error) {
	inAddr := (*inConn).RemoteAddr().String()
	inLocalAddr := (*inConn).LocalAddr().String()

	var outConn net.Conn
	localReply := true

	// 匹配跳板规则
	bridge, ok := svr.Options.Match(address)
	if ok {
		outConn, err = svr.sshPool.Dial("tcp", bridge)
		log.Println("bridge:", bridge, "host:", address)
		localReply = false
	} else {
		if geoip.InPRC(address) {
			outConn, err = net.Dial("tcp", address)
		} else {
			if len(svr.Options.Anonymous) > 0 {
				outConn, err = svr.sshPool.Dial("tcp", svr.Options.Anonymous)
				localReply = false
			} else {
				outConn, err = svr.sshPool.Dial("tcp", address)
			}
		}
	}

	if err != nil {
		log.Printf("connect to %s, err:%s", address, err)
		http.CloseConn(inConn)
		return
	}

	svr.Netflow.AddConn(1)

	if localReply && req.IsHTTPS() {
		req.HTTPSReply()
	} else {
		outConn.Write(req.HeadBuf)
	}

	svr.IoBind((*inConn), outConn, func(err error) {
		log.Printf("conn %s - %s released [%s]", inAddr, inLocalAddr, req.Host)

		http.CloseConn(inConn)
		http.CloseConn(&outConn)
	})

	log.Printf("conn %s - %s connected [%s]", inAddr, inLocalAddr, req.Host)
	return
}

func (svr *HttpServer) IoBind(src, dst net.Conn, fnClose func(err error)) {
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

func (svr *HttpServer) Shutdown() {
	svr.Listener.Close()
	svr.Netflow.Stop()
	svr.sshPool.Shutdown()
}

func (svr *HttpServer) Run() (err error) {
	if err = svr.ListenTCP(); err != nil {
		log.Println("ListenTCP:", err)
		return
	}

	return
}

func NewHttpServer(opts config.NodeConfig) (svr *HttpServer) {
	svr = new(HttpServer)
	svr.Options = opts
	return
}
