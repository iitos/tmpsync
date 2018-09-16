package main

import (
	"flag"
	"fmt"
	"os"
)

import (
	volumehelper "github.com/docker/go-plugins-helpers/volume"
)

const (
	sockAddr = "/run/docker/plugins/%v.sock"
)

func main() {
	var rootPath string

	flag.StringVar(&rootPath, "root", ".", "root directory path for tmpsync")
	flag.Parse()

	options := []string{}
	options = append(options, fmt.Sprintf("root=%s", rootPath))

	d, err := NewTmpsyncDriver(options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	h := volumehelper.NewHandler(d)
	h.ServeUnix(fmt.Sprintf(sockAddr, driverName), 0)
}
