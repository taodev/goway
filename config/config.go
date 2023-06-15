package config

import (
	"net"
	"os"
	"path/filepath"

	"github.com/taodev/go-utils"
)

type SSHConfig struct {
	URL          string `yaml:"url"`
	IdentityFile string `yaml:"identity_file"`
}

type NodeConfig struct {
	Addr      string              `yaml:"addr"`
	SSH       SSHConfig           `yaml:"ssh"`
	Anonymous string              `yaml:"anonymous"`
	Matches   map[string][]string `yaml:"matches"`
}

func (node *NodeConfig) Match(url string) (proxy string, ok bool) {
	if len(node.Matches) <= 0 {
		return
	}

	h, _, err := net.SplitHostPort(url)
	if err != nil {
		h = url
	}

	for k, v := range node.Matches {
		for _, j := range v {
			if ok, _ = filepath.Match(j, h); ok {
				proxy = k
				return
			}
		}
	}

	return
}

type Config struct {
	Addr   string                `yaml:"addr"`
	Http   map[string]NodeConfig `yaml:"http"`
	Socks5 map[string]NodeConfig `yaml:"socks5"`
	VPN    map[string]NodeConfig `yaml:"vpn"`
}

func Load(path string) (cfg *Config, err error) {
	cfg = new(Config)
	err = utils.LoadYAML(path, cfg)
	return
}

func Default(path string) (err error) {
	if len(path) <= 0 {
		path = "config.yaml"
	}

	cfg := new(Config)
	cfg.Addr = ":8000"

	cfg.Http = make(map[string]NodeConfig)
	cfg.Http["hk1"] = NodeConfig{
		Addr: ":8001",
		SSH: SSHConfig{
			URL:          "goway@localhost:22",
			IdentityFile: "./id_goway",
		},
		Anonymous: "127.0.0.1:3128",
		Matches: map[string][]string{
			"us1.godev.top:3128": []string{"*.openai.com"},
		},
	}

	cfg.Http["us1"] = NodeConfig{
		Addr: ":8002",
		SSH: SSHConfig{
			URL:          "goway@localhost:2122",
			IdentityFile: "./id_goway",
		},
	}

	if err = utils.SaveYAML(path, cfg); err != nil {
		return
	}

	err = os.Chmod(path, 0644)
	return
}
