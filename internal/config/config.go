package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Log    Log
	Addr   string
	Secret []byte
	Nodes  []string
}

type Log struct {
	Level      int
	DebugNodes int // Duration in milliseconds, skip if zero
}

func (c *Config) SecretArray() *[32]byte {
	if len(c.Secret) != 32 {
		panic("invalid secret length")
	}
	return (*[32]byte)(c.Secret)
}

func LoadFile(path string) (res *Config, err error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	res = new(Config)
	if err := json.Unmarshal(bs, res); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return
}
