package main

import (
	"flag"
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
	workingDir := flag.String("D", ".", "set working directory")
	configPath := flag.String("c", "config.yaml", "set config file")
	flag.Parse()

	if *workingDir != "" {
		_, err := os.Stat(*workingDir)
		if err != nil {
			os.MkdirAll(*workingDir, 0o777)
		}
		if err := os.Chdir(*workingDir); err != nil {
			log.Fatal(err)
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	httpServs := make([]*gohttp.HttpServer, 0, len(cfg.Http))
	for k, v := range cfg.Http {
		svr := gohttp.NewHttpServer(v)
		go func() {
			if err = svr.Run(); err != nil {
				log.Printf("start http server: %s failed", k)
				return
			}
		}()

		httpServs = append(httpServs, svr)
	}

	socksServs := make([]*socks.SocksV5Server, 0, len(cfg.Http))
	for k, v := range cfg.Socks5 {
		svr := socks.NewSocksV5Server(v)
		go func() {
			if err = svr.Run(); err != nil {
				log.Printf("start http server: %s failed", k)
				return
			}
		}()

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
