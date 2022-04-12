package internal

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

var pb *progressbar.ProgressBar

// file downloader
type downloader struct {
	concurrency int
	proxy       *url.URL
}

func (d *downloader) Download(ctx context.Context, dUrl, fileName string) error {
	if fileName == "" {
		fileName = path.Base(dUrl)
	}
	resp, err := http.DefaultClient.Head(dUrl)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 && resp.Header.Get("Accept-Ranges") == "bytes" {
		return multipartDownload(ctx, dUrl, fileName, d.concurrency, int(resp.ContentLength), d.proxy)
	} else {
		return singleDownload(ctx, dUrl, fileName, d.proxy)
	}
}

// singleDownload 单线程下载文件
func singleDownload(ctx context.Context, dUrl, fileName string, proxy *url.URL) error {
	client := newHttpClient(proxy, 10*time.Minute)
	req, err := http.NewRequest(http.MethodGet, dUrl, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	pb = progressbar.DefaultBytes(
		resp.ContentLength,
		"downloading",
	)
	defer f.Close()
	io.Copy(io.MultiWriter(f, pb), resp.Body)
	return nil
}

func NewProgressBar(max int, fileName string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(max,
		//	progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription(fmt.Sprintf("downloading %s...", fileName)),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))
	return bar
}

// getUrlHash get url hash string
func getUrlHash(dUrl string) (string, error) {
	h := md5.New()
	h.Write([]byte(dUrl))
	return hex.EncodeToString(h.Sum(nil)), nil
}
func downloadPartFile(ctx context.Context, dUrl, fileName string, index int, start, end int, proxy *url.URL) error {
	req, err := http.NewRequest("GET", dUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	client := newHttpClient(proxy, 10*time.Minute)
	resp, err := client.Do(req)
	//resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 206 {
		fmt.Println("part request status:", resp.StatusCode)
		bodyBuff, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err.Error())
		}
		fmt.Println(string(bodyBuff))
		return errors.New("invalid status")
	}
	folderName, err := getUrlHash(dUrl)

	if err != nil {
		fmt.Printf("get url hash %v \n", err.Error())
		return err
	}
	os.Mkdir(folderName, 0777)
	f, err := os.OpenFile(fmt.Sprintf("%s/part%d-%s", folderName, index, fileName), os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("OpenFile %v \n", err.Error())
		return err
	}
	defer f.Close()
	_, err = io.Copy(io.MultiWriter(f, pb), resp.Body)

	if err != nil {
		if err == io.EOF {
			return nil
		}
		if err != nil {
			fmt.Printf("io Copy %v \n", err.Error())
			return err
		}
		return err
	}
	return nil
}
func newHttpClient(proxy *url.URL, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			Proxy: http.ProxyURL(proxy),
		},
	}
}

// multipartDownload 并发下载文件
func multipartDownload(ctx context.Context, dUrl, fileName string, concurrency, size int, proxy *url.URL) error {
	partSize := size / concurrency
	if pb == nil {
		pb = NewProgressBar(size, fileName)
	}
	wg := sync.WaitGroup{}
	wg.Add(concurrency)
	start, end := 0, partSize
	for i := 0; i < concurrency; i++ {
		if end > size {
			end = size
		}
		go func(fno, s, e int) {
			err := downloadPartFile(ctx, dUrl, fileName, fno, s, e, proxy)
			if err != nil {
				log.Fatalf("download part file error:%v \n", err)
			}
			wg.Done()
		}(i, start, end)
		start = end + 1
		end += partSize

	}
	wg.Wait()
	err := megePartFiles(dUrl, fileName)
	if err != nil {
		log.Fatalf("%v", err)
	}
	pb.Finish()
	return nil
}
func megePartFiles(dUrl string, fileName string) error {
	folderName, err := getUrlHash(dUrl)
	if err != nil {
		return err
	}
	dirEntries, err := os.ReadDir(folderName)
	if err != nil {
		return err
	}
	fdest, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer fdest.Close()
	parts := make([]string, 0)
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		parts = append(parts, entry.Name())
	}
	sort.Strings(parts)
	for _, part := range parts {
		fName := fmt.Sprintf("%s/%s", folderName, part)
		f, err := os.OpenFile(fName, os.O_RDONLY, 0666)
		if err != nil {
			return err
		}
		_, err = io.Copy(fdest, f)
		if err != nil {
			return err
		}
		f.Close()
	}
	err = os.RemoveAll(folderName)
	if err != nil {
		return err
	}
	return nil
}

func NewDownloader(concurrency int, proxy string) *downloader {
	if concurrency < 1 {
		concurrency = runtime.NumCPU()
	}
	if proxy != "" {
		proxyUrl, err := url.Parse(proxy)
		if err != nil {
			panic(err)
		}
		return &downloader{
			concurrency, proxyUrl,
		}
	} else {
		return &downloader{
			concurrency, nil,
		}
	}

}
