package main

import (
	"flag"
)

var flagRunAddr string
var flagUrlAddr string

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")
	flag.StringVar(&flagUrlAddr, "b", "http://localhost:8080/", "address and port short url")
	flag.Parse()
}
