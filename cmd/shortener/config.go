package main

import (
	"flag"
	"os"
	"time"
)

var flagRunAddr string
var flagShortAddr string
var flagFileBD string
var flagConnDB string
var flagEnableAuth bool
var flagDeleteBufferSize int = 100
var flagDeleteFlushInterval time.Duration = 5 * time.Second

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")
	flag.StringVar(&flagShortAddr, "b", "http://localhost:8080", "address and port short url")
	flag.StringVar(&flagFileBD, "f", "file_bd.txt", "file bd")
	flag.StringVar(&flagConnDB, "d", "", "file bd")
	flag.BoolVar(&flagEnableAuth, "auth", true, "enable authentication")
	flag.IntVar(&flagDeleteBufferSize, "dbs", 100, "Delete buffer size")
	flag.DurationVar(&flagDeleteFlushInterval, "dfi", 5*time.Second, "Delete flush interval")

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

	if envConnDB := os.Getenv("DATABASE_DSN"); envConnDB != "" {
		flagConnDB = envConnDB
	}

	if envEnableAuth := os.Getenv("ENABLE_AUTH"); envEnableAuth == "true" {
		flagEnableAuth = true
	}

}
