package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	conf := Config{
		Addr:   "localhost:8909",
		Secret: []byte("hello"),
	}
	assert.PanicsWithValue(
		t, "invalid secret length",
		func() {
			conf.SecretArray()
		},
	)
}
