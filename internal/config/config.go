package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server Server
	Log    Log
}

type Server struct {
	Addr   string
	Secret string
	Nodes  []string
}

func (c *Server) SecretArray() *[32]byte {
	bs := []byte(c.Secret)
	return (*[32]byte)(bs)
}

func (c *Server) Validate() error {
	if l := len(c.Secret); l != 32 {
		return fmt.Errorf("secret length should be 32, got: %d", l)
	}
	return nil
}

type Log struct {
	Level      int
	DebugNodes int // Duration in milliseconds, skip if zero
}

func LoadFile(path string) (res *Config, err error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	res = new(Config)
	if err := toml.Unmarshal(bs, res); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if err := res.Server.Validate(); err != nil {
		return nil, fmt.Errorf("validate server: %w", err)
	}

	return
}
