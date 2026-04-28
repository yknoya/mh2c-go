package main

import (
	"io"
	"os"
)

func prepareOutputWriter(stdout io.Writer, savePath string) (io.Writer, func() error, error) {
	if savePath == "" {
		return stdout, func() error { return nil }, nil
	}
	file, err := os.Create(savePath)
	if err != nil {
		return nil, nil, err
	}
	return io.MultiWriter(stdout, file), file.Close, nil
}
