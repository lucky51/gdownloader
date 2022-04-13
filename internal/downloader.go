package internal

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

var pb *progressbar.ProgressBar

type GDVersion struct {
	Major byte
	Minor byte
	Patch byte
}

func (v *GDVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// file downloader
type downloader struct {
	concurrency int
	proxy       *url.URL
	retry       int
	timeout     time.Duration
}

func (d *downloader) Download(ctx context.Context, dUrl, fileName string) error {
	if fileName == "" {
		fileName = path.Base(dUrl)
		fUrl, err := url.Parse(fileName)
		if err != nil {
			hashStr, err := getUrlHash(dUrl)
			if err != nil {
				fileName = fmt.Sprintf("%d", time.Now().UnixNano())
			} else {
				fileName = hashStr
			}
		} else {
			fileName = fUrl.Path
		}
	}
	resp, err := http.DefaultClient.Head(dUrl)
	if err != nil {
		return err
	}
	contentType := strings.ToUpper(resp.Header.Get("Content-Type"))
	fmt.Println("content-type:", contentType)

	if resp.StatusCode == 200 &&
		resp.Header.Get("Accept-Ranges") == "bytes" && resp.ContentLength > 1024*100 {
		return d.multipartDownload(ctx, dUrl, fileName, resp.ContentLength)
	} else {
		ctx, cancelFunc := context.WithTimeout(ctx, d.timeout)
		defer cancelFunc()
		return singleDownload(ctx, dUrl, fileName, d.proxy)
	}
}

// singleDownload 单线程下载文件
func singleDownload(ctx context.Context, dUrl, fileName string, proxy *url.URL) error {
	client := newHttpClient(proxy)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dUrl, nil)
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

func newProgressBar(max int64, fileName string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions64(max,
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
func genPartPath(folderName string, partIndex int, fileName string, start, end int64) string {
	fileName = strings.ReplaceAll(fileName, ".", "_")
	return fmt.Sprintf("%s/part%d-%s-%d-%d", folderName, partIndex, fileName, start, end)
}
func downloadPartFile(ctx context.Context, dUrl, filePath string, index int, start, end int64, proxy *url.URL) error {
	req, err := http.NewRequestWithContext(ctx, "GET", dUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	client := newHttpClient(proxy)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 206 {
		fmt.Println("part request status:", resp.StatusCode)
		return errors.New("invalid status")
	}
	//partPath := genPartPath(folderName, index, fileName, start, end)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND, 0666)

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
func newHttpClient(proxy *url.URL) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			Proxy: http.ProxyURL(proxy),
		},
	}
}
func deleteExistUnCompletedPart(fPath string) bool {
	fInfo, err := os.Stat(fPath)
	if err != nil {
		return false
	}
	if fInfo.IsDir() {
		return false
	}
	subs := strings.Split(fInfo.Name(), "-")
	lt, err := strconv.ParseInt(subs[len(subs)-2], 10, 64)
	if err != nil {
		fmt.Println(err)
	}
	rh, err := strconv.ParseInt(subs[len(subs)-1], 10, 64)
	if err != nil {
		fmt.Println(err)
	}
	if fInfo.Size() != rh-lt+1 {
		fmt.Printf("remove part file:%s,file size:%d,%d-%d,expected:%d \n", fInfo.Name(), fInfo.Size(), rh, lt, rh-lt+1)
		os.Remove(fPath)
		return false
	} else {
		return true
	}
}

// multipartDownload 并发下载文件
func (d *downloader) multipartDownload(ctx context.Context, dUrl, fileName string, size int64) error {
	partSize := size / int64(d.concurrency)
	if pb == nil {
		pb = newProgressBar(size, fileName)
	}
	folderName, err := getUrlHash(dUrl)
	if err != nil {
		fmt.Printf("get url hash %v \n", err.Error())
		return err
	}
	os.Mkdir(folderName, 0777)
	wg := sync.WaitGroup{}
	//errWg := errgroup.Group{}
	wg.Add(d.concurrency)
	var start, end int64 = 0, partSize
	for i := 0; i < d.concurrency; i++ {
		if end > size {
			end = size
		}
		partPath := genPartPath(folderName, i, fileName, start, end)
		if !deleteExistUnCompletedPart(partPath) {
			go func(fno int, s, e int64) {
			retry:
				ctx, cancelFunc := context.WithTimeout(ctx, d.timeout)
				defer cancelFunc()
				err := downloadPartFile(ctx, dUrl, partPath, fno, s, e, d.proxy)
				var r float64 = 0
				if err != nil {
					log.Printf("download part file %s error:%v \n", partPath, err)
					if int(r) < d.retry {
						nextReq := time.Duration(math.Pow(2, r))
						fmt.Printf("retry after %d sec. \n", int(math.Pow(2, r)))
						<-time.After(time.Second * nextReq)
						r++
						os.Remove(partPath)
						goto retry
					}
					log.Fatalln("download failed!")
				}
				wg.Done()
			}(i, start, end)
		} else {
			pb.ChangeMax64(size - (end - start + 1))
			wg.Done()
		}
		start = end + 1
		end += partSize
	}
	wg.Wait()
	err = megePartFiles(dUrl, fileName)
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
	fdest, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
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
	// err = os.RemoveAll(folderName)
	// if err != nil {
	// 	return err
	// }
	return nil
}

func NewDownloader(concurrency int, retry int, timeout time.Duration, proxy string) *downloader {
	if concurrency < 1 {
		concurrency = runtime.NumCPU()
	}
	if proxy != "" {
		proxyUrl, err := url.Parse(proxy)
		if err != nil {
			panic(err)
		}
		return &downloader{
			concurrency, proxyUrl, retry, timeout,
		}
	} else {
		return &downloader{
			concurrency, nil, retry, timeout,
		}
	}

}
