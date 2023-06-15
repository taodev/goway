package myssh

import (
	"errors"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const DEFAULT_TIMEOUT = 3 * time.Minute

var (
	ErrNotValid = errors.New("SSHClient not valid")
)

type SSHConn struct {
	net.Conn
	IsLocal bool
}

func (c *SSHConn) Read(b []byte) (n int, err error) {
	c.SetReadDeadline(time.Now().Add(DEFAULT_TIMEOUT))
	return c.Conn.Read(b)
}

func (c *SSHConn) Write(b []byte) (n int, err error) {
	c.SetWriteDeadline(time.Now().Add(DEFAULT_TIMEOUT))
	return c.Conn.Write(b)
}

func (c *SSHConn) Close() error {
	c.SetDeadline(time.Now().Add(DEFAULT_TIMEOUT))
	return c.Conn.Close()
}

type SSHClient struct {
	Addr    string
	User    string
	KeyFile string

	c *ssh.Client

	closeOnce *sync.Once

	chDial     chan int
	chShutdown chan int

	wg     sync.WaitGroup
	locker sync.RWMutex
}

func (cli *SSHClient) Dial(n string, addr string) (c net.Conn, err error) {
	if !cli.IsValid() {
		err = ErrNotValid
		return
	}

	c, err = cli.c.Dial(n, addr)
	if err == io.EOF {
		log.Println("[ERROR] ssh: connect failed")
		cli.chDial <- 0
	}

	c = &SSHConn{
		Conn: c,
	}

	return
}

func (cli *SSHClient) Close() {
	if !cli.IsValid() {
		return
	}

	cli.closeOnce.Do(func() {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("SSHClient::Close crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
			}
		}()

		cli.locker.Lock()
		defer cli.locker.Unlock()

		cli.c.Close()
		cli.c = nil
		cli.chDial = nil
	})
}

func (cli *SSHClient) Shutdown() {
	cli.chShutdown <- 0
	cli.wg.Wait()
	cli.Close()
}

func (cli *SSHClient) Ping() (err error) {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("SSHClient::Ping crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
		}
	}()

	if cli.c == nil {
		err = ErrNotValid
		return
	}

	_, _, err = cli.c.SendRequest("keepalive@ssh-tunnel", true, nil)
	return
}

func (cli *SSHClient) IsValid() bool {
	cli.locker.RLock()
	defer cli.locker.RUnlock()

	return cli.c != nil
}

func (cli *SSHClient) dial() (err error) {
	pemFile, err := os.ReadFile(cli.KeyFile)
	if err != nil {
		return
	}

	signer, err := ssh.ParsePrivateKey(pemFile)
	if err != nil {
		log.Printf("ssh: %v", err)
		return
	}

	conf := &ssh.ClientConfig{
		User:            cli.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		Timeout:         3 * time.Minute,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		BannerCallback: func(message string) error {
			log.Println("[ERROR] ssh:", message)
			return nil
		},
	}

	if cli.c, err = ssh.Dial("tcp", cli.Addr, conf); err != nil {
		return
	}

	cli.closeOnce = new(sync.Once)

	return
}

func (cli *SSHClient) keeplive() {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("SSHClient::keeplive crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
		}
	}()

	cli.wg.Add(1)
	defer cli.wg.Done()

	ticker := time.NewTicker(time.Minute * 1)

	log.Println("ssh: keeplive start")

	fn := func() (err error) {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("SSHClient::ping crashed , err : %s , \ntrace:%s", e, string(debug.Stack()))
			}
		}()

		if err = cli.Ping(); err != nil {
			cli.Close()
			if err = cli.dial(); err != nil {
				log.Println("ssh: reconnect failed.")
			} else {
				log.Println("ssh: reconnect success.")
			}
		}

		return
	}

	running := true
	for running {
		select {
		case <-cli.chDial:
			fn()
		case <-ticker.C:
			fn()
		case <-cli.chShutdown:
			running = false
		}
	}

	log.Println("ssh: keeplive exit.")
}

func NewSSHClient(remoteURL string, remoteKey string) (cli *SSHClient, err error) {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return
	}

	cli = new(SSHClient)
	cli.Addr = u.Hostname() + ":" + u.Port()
	cli.User = u.User.Username()
	cli.KeyFile = remoteKey
	cli.chDial = make(chan int)
	cli.chShutdown = make(chan int)

	if err = cli.dial(); err != nil {
		cli = nil
		return
	}

	go cli.keeplive()

	return
}
