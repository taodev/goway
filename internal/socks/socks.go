package socks

import (
	"errors"
	"fmt"
	"net"
	"time"
)

var (
	ErrSocks5HandshakeRequest = errors.New("socks5 handshake request error")
	ErrSocks5Request          = errors.New("socks5 request error")
)

func Socks5Handshake(conn net.Conn) (err error) {
	// socks5 handshake request
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	// 二进制打印协议
	fmt.Printf("n: %d, %x\n", n, buf[:n])

	// socks5 handshake request
	if n < 3 || buf[0] != 0x05 || buf[1] != 0x01 || buf[2] != 0x00 {
		err = ErrSocks5HandshakeRequest
		return
	}

	// socks5 handshake response
	_, err = conn.Write([]byte{0x05, 0x00})
	if err != nil {
		return
	}

	// // socks5 handshake response
	// n, err = conn.Read(buf)
	// if err != nil {
	// 	return
	// }

	// // 二进制打印协议
	// fmt.Printf("n: %d, %x\n", n, buf[:n])
	// // socks5 handshake response
	// if n < 2 || buf[0] != 0x05 || buf[1] != 0x01 {
	// 	err = ErrSocks5HandshakeRequest
	// 	return
	// }

	return
}

type Socks5RequestData struct {
	Ver      byte
	Cmd      byte
	Rsv      byte
	Atyp     byte
	DstAddr  net.IP
	DstPort  uint16
	Host     string
	Username string
	Password string
	Request  []byte
}

func Socks5Request(conn net.Conn) (req *Socks5RequestData, err error) {
	// socks5 request
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	// 二进制打印协议
	fmt.Printf("n: %d, %x\n", n, buf[:n])

	// socks5 request
	if n < 10 || buf[0] != 0x05 || buf[1] != 0x01 {
		err = ErrSocks5Request
		return
	}

	// socks5 request
	req = &Socks5RequestData{
		Ver:     buf[0],
		Cmd:     buf[1],
		Rsv:     buf[2],
		Atyp:    buf[3],
		Request: buf[:n],
	}

	// socks5 request
	switch req.Atyp {
	case 0x01:
		req.DstAddr = net.IP(buf[4 : 4+net.IPv4len])
	case 0x03:
		fmt.Printf("len: %d %s\n", buf[4], buf[5:5+buf[4]])
		// req.DstAddr = net.IP(buf[5 : 5+buf[4]])
		req.Host = string(buf[5 : 5+buf[4]])
	case 0x04:
		req.DstAddr = net.IP(buf[4 : 4+net.IPv6len])
	}

	// socks5 request
	req.DstPort = uint16(buf[n-2])<<8 | uint16(buf[n-1])

	return
}

func Socks5Response(conn net.Conn, req *Socks5RequestData) (err error) {
	// socks5 response
	var resp []byte
	switch req.Atyp {
	case 0x01:
		resp = []byte{0x05, 0x00, 0x00, 0x01}
	case 0x03:
		resp = []byte{0x05, 0x00, 0x00, 0x03, byte(len(req.Host))}
	case 0x04:
		resp = []byte{0x05, 0x00, 0x00, 0x04}
	}

	// socks5 response
	resp = append(resp, req.DstAddr...)
	resp = append(resp, byte(req.DstPort>>8), byte(req.DstPort))

	// socks5 response
	_, err = conn.Write(resp)
	if err != nil {
		return
	}

	return
}

func Socks5Connect(conn net.Conn, req *Socks5RequestData) (err error) {
	// socks5 connect
	var dstAddr string
	switch req.Atyp {
	case 0x01:
		dstAddr = fmt.Sprintf("%s:%d", req.DstAddr, req.DstPort)
	case 0x03:
		dstAddr = fmt.Sprintf("%s:%d", req.Host, req.DstPort)
	case 0x04:
		dstAddr = fmt.Sprintf("[%s]:%d", req.DstAddr, req.DstPort)
	}

	// socks5 connect
	dstConn, err := net.DialTimeout("tcp", dstAddr, 10*time.Second)
	if err != nil {
		return
	}

	// socks5 connect
	defer dstConn.Close()

	// socks5 connect
	if req.Cmd == 0x01 {
		// socks5 connect
		_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01})
		if err != nil {
			return
		}
	}

	return
}
