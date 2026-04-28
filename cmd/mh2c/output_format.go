package main

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode/utf8"
)

func truncateHex(data []byte, limit uint) (string, bool) {
	if limit == 0 || len(data) <= int(limit) {
		return hex.EncodeToString(data), false
	}
	return fmt.Sprintf("%s...(truncated %d/%d bytes)", hex.EncodeToString(data[:limit]), limit, len(data)), true
}

func formatHexSummary(data []byte, limit uint) string {
	text, _ := truncateHex(data, limit)
	return text
}

func formatDataTextLimited(data []byte, limit uint) string {
	text, _ := formatDataTextJSON(data, limit)
	return text
}

func formatDataTextJSON(data []byte, limit uint) (string, bool) {
	if len(data) == 0 {
		return "<empty>", false
	}

	truncated := false
	if limit > 0 && len(data) > int(limit) {
		data = truncateUTF8Prefix(data[:limit])
		truncated = true
	}

	if utf8.Valid(data) {
		text := strconv.Quote(string(data))
		if truncated {
			text += " (truncated)"
		}
		return text, truncated
	}
	return "<non-utf8>", truncated
}

func truncateUTF8Prefix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for len(data) > 0 {
		data = data[:len(data)-1]
		if utf8.Valid(data) {
			return data
		}
	}
	return nil
}
