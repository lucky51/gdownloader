package main

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/lucky51/gdownloader/internal"
	"github.com/spf13/cobra"
)

var proxy string
var dUrl string
var concurrency int

var rootCMD = &cobra.Command{
	Short:   "a simple downloader",
	Version: "v0.1.0",
	Run: func(cmd *cobra.Command, args []string) {
		if dUrl == "" {
			cmd.Help()
			log.Fatal("request url is required \n")
		}
		_, err := url.Parse(dUrl)
		if err != nil {
			fmt.Println("invalid url:", dUrl)
			return
		}
		dw := internal.NewDownloader(concurrency, proxy)
		dw.Download(context.Background(), dUrl, "")
	},
}

func init() {
	rootCMD.Flags().StringVarP(&proxy, "proxy", "p", "", "request proxy")
	rootCMD.Flags().StringVarP(&dUrl, "url", "u", "", "request url")
	rootCMD.Flags().IntVarP(&concurrency, "concurrency", "c", 0, "concurrency")
}
func main() {
	err := rootCMD.Execute()
	if err != nil {
		panic(err)
	}
}
