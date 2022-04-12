package main

import (
	"context"
	"flag"
	"log"

	"github.com/lucky51/gdownloader/internal"
)

var proxy string
var url string
var concurrency int

func main() {
	flag.StringVar(&proxy, "proxy", "", "request proxy")
	flag.StringVar(&url, "url", "", "request url")
	flag.IntVar(&concurrency, "concurrency", 0, "")
	flag.Parse()
	if url == "" {
		log.Fatal("request url is required \n")
	}
	dw := internal.NewDownloader(concurrency, proxy)
	dw.Download(context.Background(), url, "")
}
