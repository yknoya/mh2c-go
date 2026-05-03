package framefmt

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode/utf8"
)

const (
	DataFormatText = "text"
	DataFormatHex  = "hex"
	DataFormatBoth = "both"
)

func TruncateHex(data []byte, limit uint) (string, bool) {
	if limit == 0 || len(data) <= int(limit) {
		return hex.EncodeToString(data), false
	}
	return fmt.Sprintf("%s...(truncated %d/%d bytes)", hex.EncodeToString(data[:limit]), limit, len(data)), true
}

func HexSummary(data []byte, limit uint) string {
	text, _ := TruncateHex(data, limit)
	return text
}

func DataTextLimited(data []byte, limit uint) string {
	text, _ := DataTextJSON(data, limit)
	return text
}

func DataTextJSON(data []byte, limit uint) (string, bool) {
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
