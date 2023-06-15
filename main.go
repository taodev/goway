package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/taodev/goway/config"
	gohttp "github.com/taodev/goway/services/http"
	"github.com/taodev/goway/services/socks"
)

func main() {
	// config.Default("config.yaml")
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	hk1 := cfg.Http["hk1"]
	matches := []string{
		"chat.openai.com",
		"openai.com",
		"google.com",
		"chat.openai.com:443",
		"openai.com:443",
		"google.com:443",
	}

	for _, v := range matches {
		proxy, ok := hk1.Match(v)
		log.Println(v, proxy, ok)
	}

	fmt.Println(cfg)

	httpServs := make([]*gohttp.HttpServer, 0, len(cfg.Http))
	for k, v := range cfg.Http {
		svr := gohttp.NewHttpServer(v)
		if err = svr.Run(); err != nil {
			log.Printf("start http server: %s failed", k)
			return
		}

		httpServs = append(httpServs, svr)
	}

	socksServs := make([]*socks.SocksV5Server, 0, len(cfg.Http))
	for k, v := range cfg.Socks5 {
		svr := socks.NewSocksV5Server(v)
		if err = svr.Run(); err != nil {
			log.Printf("start http server: %s failed", k)
			return
		}

		socksServs = append(socksServs, svr)
	}

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(signalChan,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	go func() {
		for range signalChan {
			fmt.Println("\nReceived an interrupt, stopping services...")
			cleanupDone <- true
		}
	}()

	<-cleanupDone

	for _, v := range httpServs {
		v.Shutdown()
	}

	for _, v := range socksServs {
		v.Shutdown()
	}
}
