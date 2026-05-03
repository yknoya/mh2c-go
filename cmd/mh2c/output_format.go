package main

import "github.com/yknoya/mh2c-go/internal/framefmt"

func truncateHex(data []byte, limit uint) (string, bool) {
	return framefmt.TruncateHex(data, limit)
}

func formatDataTextJSON(data []byte, limit uint) (string, bool) {
	return framefmt.DataTextJSON(data, limit)
}
