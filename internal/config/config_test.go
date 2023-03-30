package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	s := Server{
		Addr:   "localhost:8909",
		Secret: "hello",
	}
	assert.Panics(t, func() {
		s.SecretArray()
	})

	s.Secret = "abcdefghijklmnopqrstuvwxyz1234567890"
	assert.NotPanics(t, func() {
		_ = (*[32]byte)([]byte(s.Secret))
	})
}
