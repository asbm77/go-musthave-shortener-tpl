package main

import (
	"flag"
	"os"
)

var flagRunAddr string
var flagShortAddr string
var flagFileBD string

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")
	flag.StringVar(&flagShortAddr, "b", "http://localhost:8080", "address and port short url")
	flag.StringVar(&flagFileBD, "f", "file_bd.txt", "file bd")
	flag.Parse()

	if envRunAddr := os.Getenv("SERVER_ADDRESS"); envRunAddr != "" {
		flagRunAddr = envRunAddr
	}

	if envShortAddr := os.Getenv("BASE_URL"); envShortAddr != "" {
		flagShortAddr = envShortAddr
	}

	if envFileBD := os.Getenv("FILE_STORAGE_PATH"); envFileBD != "" {
		flagFileBD = envFileBD
	}
}
