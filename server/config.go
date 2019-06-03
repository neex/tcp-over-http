package server

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	ListenAddr  string        `yaml:"listen_addr"`
	Token       string        `yaml:"token"`
	StaticDir   string        `yaml:"static_dir"`
	Domain      string        `yaml:"domain"`
	CertPath    string        `yaml:"cert_path"`
	KeyPath     string        `yaml:"key_path"`
	DialTimeout time.Duration `yaml:"dial_timeout"`

	Certificate tls.Certificate `yaml:"-"`
}

func NewConfigFromFile(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	dec := yaml.NewDecoder(f)
	cfg := &Config{}
	if err := dec.Decode(cfg); err != nil {
		return nil, err
	}

	if !cfg.IsHTTPS() {
		log.Printf("Warning: serving without https")
	} else {
		cfg.Certificate, err = tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (c *Config) IsHTTPS() bool {
	return c.CertPath != ""
}
