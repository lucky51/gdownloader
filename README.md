# gdownloader

a simple downloader

## Usage 

```shell
go install github.com/lucky51/gdownloader/cli/gd@latest
gd --url https://download-cdn.jetbrains.com/go/goland-2021.3.4.exe --proxy socks5://127.0.0.1:10808 --timeout 10m --retry 3

```

for windows ANSI Color 

[win10colors.cmd by mlocati](https://gist.github.com/mlocati/fdabcaeb8071d5c75a2d51712db24011)

## Related Projects

* [cobra A Commander for modern Go CLI interactions](https://github.com/spf13/cobra)
* [A really basic thread-safe progress bar for Golang applications](https://github.com/schollz/progressbar)