package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/go-logr/stdr"

	"github.com/adobaai/bifrost/internal/config"
	"github.com/adobaai/bifrost/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Println("ERR:", err)
	}
}

func run() error {
	l := stdr.New(log.Default())
	conf, err := config.LoadFile("config.toml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stdr.SetVerbosity(conf.Log.Level)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	server := server.New(conf, l)
	go func() {
		<-c
		if err := server.Stop(); err != nil {
			l.Error(err, "stop server")
		}
	}()
	return server.Start()
}
