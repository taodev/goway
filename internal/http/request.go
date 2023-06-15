package http

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

func CloseConn(conn *net.Conn) {
	if conn != nil && *conn != nil {
		(*conn).SetDeadline(time.Now().Add(time.Millisecond))
		(*conn).Close()
	}
}

type HTTPRequest struct {
	HeadBuf   []byte
	conn      *net.Conn
	Host      string
	Method    string
	URL       string
	hostOrURL string
}

func NewHTTPRequest(inConn *net.Conn, bufSize int) (req HTTPRequest, err error) {
	buf := make([]byte, bufSize)
	len := 0
	req = HTTPRequest{
		conn: inConn,
	}
	len, err = (*inConn).Read(buf[:])
	if err != nil {
		if err != io.EOF {
			err = fmt.Errorf("http decoder read err:%s", err)
		}
		CloseConn(inConn)
		return
	}
	req.HeadBuf = buf[:len]
	index := bytes.IndexByte(req.HeadBuf, '\n')
	if index == -1 {
		err = fmt.Errorf("http decoder data line err:%s", string(req.HeadBuf)[:50])
		CloseConn(inConn)
		return
	}
	fmt.Sscanf(string(req.HeadBuf[:index]), "%s%s", &req.Method, &req.hostOrURL)
	if req.Method == "" || req.hostOrURL == "" {
		err = fmt.Errorf("http decoder data err:%s", string(req.HeadBuf)[:50])
		CloseConn(inConn)
		return
	}
	req.Method = strings.ToUpper(req.Method)
	// log.Printf("%s:%s", req.Method, req.hostOrURL)

	if req.IsHTTPS() {
		err = req.HTTPS()
	} else {
		err = req.HTTP()
	}
	return
}

func (req *HTTPRequest) HTTP() (err error) {
	req.URL, err = req.getHTTPURL()
	if err == nil {
		u, _ := url.Parse(req.URL)
		req.Host = u.Host
		req.addPortIfNot()
	}
	return
}

func (req *HTTPRequest) HTTPS() (err error) {
	req.Host = req.hostOrURL
	req.addPortIfNot()
	//_, err = fmt.Fprint(*req.conn, "HTTP/1.1 200 Connection established\r\n\r\n")
	return
}

func (req *HTTPRequest) HTTPSReply() (err error) {
	_, err = fmt.Fprint(*req.conn, "HTTP/1.1 200 Connection established\r\n\r\n")
	return
}

func (req *HTTPRequest) IsHTTPS() bool {
	return req.Method == "CONNECT"
}

func (req *HTTPRequest) getHTTPURL() (URL string, err error) {
	if !strings.HasPrefix(req.hostOrURL, "/") {
		return req.hostOrURL, nil
	}
	_host, err := req.getHeader("host")
	if err != nil {
		return
	}
	URL = fmt.Sprintf("http://%s%s", _host, req.hostOrURL)
	return
}

func (req *HTTPRequest) getHeader(key string) (val string, err error) {
	key = strings.ToUpper(key)
	lines := strings.Split(string(req.HeadBuf), "\r\n")
	for _, line := range lines {
		line := strings.SplitN(strings.Trim(line, "\r\n "), ":", 2)
		if len(line) == 2 {
			k := strings.ToUpper(strings.Trim(line[0], " "))
			v := strings.Trim(line[1], " ")
			if key == k {
				val = v
				return
			}
		}
	}
	err = fmt.Errorf("can not find HOST header")
	return
}

func (req *HTTPRequest) addPortIfNot() (newHost string) {
	//newHost = req.Host
	port := "80"
	if req.IsHTTPS() {
		port = "443"
	}

	// if (!strings.HasPrefix(req.Host, "[") && strings.Index(req.Host, ":") == -1) || (strings.HasPrefix(req.Host, "[") && strings.HasSuffix(req.Host, "]")) {
	if (!strings.HasPrefix(req.Host, "[") && !strings.Contains(req.Host, ":")) || (strings.HasPrefix(req.Host, "[") && strings.HasSuffix(req.Host, "]")) {
		//newHost = req.Host + ":" + port
		//req.headBuf = []byte(strings.Replace(string(req.headBuf), req.Host, newHost, 1))
		req.Host = req.Host + ":" + port
	}
	return
}
