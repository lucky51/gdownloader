package main

import (
	"context"
	"fmt"
	"github.com/lucky51/gdownloader/internal"
	"github.com/spf13/cobra"
	"gopkg.in/elazarl/goproxy.v1"
	"log"
	"net/http"
	"net/url"
	"time"
)

var proxyUrl string
var dUrl string
var concurrency int
var retry int
var timeout time.Duration
var rootCMD = &cobra.Command{
	Short:   "a simple downloader",
	Version: "v0.1.3",
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
		dw := internal.NewDownloader(concurrency, retry, timeout, proxyUrl)
		dw.Download(context.Background(), dUrl, "")
	},
}
var listenPort int
var proxyCMD = &cobra.Command{
	Use:   "bridge",
	Short: "start http/https proxy",
	Run: func(cmd *cobra.Command, args []string) {
		p := goproxy.NewProxyHttpServer()
		p.Verbose = true
		if proxyUrl != "" {
			u, err := url.Parse(proxyUrl)
			if err != nil {
				cmd.PrintErrln("proxy url is invalid")
				return
			}
			if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "" {
				cmd.PrintErrln("only support scheme http or https")
				return
			}
			p.ConnectDial = p.NewConnectDialToProxy(proxyUrl)
		}
		fmt.Println("start listening port:", listenPort)
		log.Fatalln(http.ListenAndServe(fmt.Sprintf(":%d", listenPort), p))
	},
}

func init() {
	rootCMD.Flags().StringVarP(&proxyUrl, "proxy", "p", "", "proxy url")
	rootCMD.Flags().StringVarP(&dUrl, "url", "u", "", "request url")
	rootCMD.Flags().IntVarP(&concurrency, "concurrency", "c", 0, "concurrency ,default runtime.NumCPU")
	rootCMD.Flags().IntVarP(&retry, "retry", "r", 3, "retry times")
	rootCMD.Flags().DurationVarP(&timeout, "timeout", "t", 0, "request timeout ，default no timeout")
	rootCMD.AddCommand(proxyCMD)
	proxyCMD.Flags().IntVarP(&listenPort, "port", "p", 8089, "listen port")
	proxyCMD.Flags().StringVarP(&proxyUrl, "proxy", "P", "", "proxy url")
}
func main() {
	err := rootCMD.Execute()
	if err != nil {
		panic(err)
	}
}
