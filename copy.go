package main

import (
	"log"

	"github.com/codegangsta/cli"
)

func ActionCopy(c *cli.Context) {
	if len(c.Args()) < 1 {
		log.Fatal("usage: hookbot copy <ws://token@host:2134/sub/[topic]>")
	}

	log.Fatal("Not yet implemented")
}
