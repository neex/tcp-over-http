package client

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Address                string        `yaml:"address"`
	RemoteTimeout          time.Duration `yaml:"remote_timeout"`
	ConnectTimeout         time.Duration `yaml:"connect_timeout"`
	KeepAliveTimeout       time.Duration `yaml:"keep_alive_timeout"`
	MaxConnectionMultiplex int           `yaml:"max_connection_multiplex"`
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

	return cfg, nil
}
