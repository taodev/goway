package netflow

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type NetflowConn struct {
	net.Conn
	Netflow *Netflow
}

func (c *NetflowConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if c.Netflow != nil {
		c.Netflow.Report(n, 0)
	}
	return
}

func (c *NetflowConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if c.Netflow != nil {
		c.Netflow.Report(0, n)
	}
	return
}

type NetflowInfo struct {
	ConnTotal    int32
	ReadTotal    int64
	WrittenTotal int64
	ReadSpeed    int64
	WrittenSpeed int64
}

type Netflow struct {
	readTotal    int64
	writtenTotal int64
	connTotal    int32
	stopCH       chan int
}

func (nf *Netflow) AddConn(n int32) {
	atomic.AddInt32(&nf.connTotal, int32(n))
}

func (nf *Netflow) DelConn(n int32) {
	atomic.AddInt32(&nf.connTotal, int32(-n))
}

func (nf *Netflow) Report(r, w int) {
	if r > 0 {
		atomic.AddInt64(&nf.readTotal, int64(r))
	}

	if w > 0 {
		atomic.AddInt64(&nf.writtenTotal, int64(w))
	}
}

func (nf *Netflow) ConnTotal() (n int32) {
	n = atomic.LoadInt32(&nf.connTotal)
	return
}

func (nf *Netflow) ReadTotal() (n int64) {
	n = atomic.LoadInt64(&nf.readTotal)
	return
}

func (nf *Netflow) WrittenTotal() (n int64) {
	n = atomic.LoadInt64(&nf.writtenTotal)
	return
}

func (nf *Netflow) Start(sec int, fnlog func(i NetflowInfo)) {
	nf.stopCH = make(chan int)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		logTime := 0

		var r, w int64

		var i NetflowInfo

		running := true
		for running {
			select {
			case <-ticker.C:
				logTime += 1

				i.ConnTotal = nf.ConnTotal()
				i.ReadTotal = nf.ReadTotal()
				i.WrittenTotal = nf.WrittenTotal()

				if i.ReadTotal-r > i.ReadSpeed {
					i.ReadSpeed = i.ReadTotal - r
				}

				if i.WrittenTotal-w > i.WrittenSpeed {
					i.WrittenSpeed = i.WrittenTotal - w
				}

				if logTime >= sec {
					fnlog(i)

					logTime = 0
					i.ReadSpeed = 0
					i.WrittenSpeed = 0
				}

				r = i.ReadTotal
				w = i.WrittenTotal
			case <-nf.stopCH:
				running = false
			}
		}
	}()
}

func (nf *Netflow) Stop() {
	nf.stopCH <- 0
}

func BytesFormat(v int64) string {
	if v > 1024*1024 {
		return fmt.Sprintf("%.2fMB", float64(v)/(1024*1024))
	}

	if v > 1024 {
		return fmt.Sprintf("%.2fKB", float64(v)/1024)
	}

	return fmt.Sprintf("%dB", v)
}
