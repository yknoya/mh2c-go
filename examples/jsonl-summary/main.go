package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type frameEvent struct {
	Direction      string        `json:"direction"`
	FrameType      string        `json:"frame_type"`
	StreamID       uint32        `json:"stream_id"`
	Summary        string        `json:"summary"`
	DataText       string        `json:"data_text"`
	DecodedHeaders []headerField `json:"decoded_headers"`
}

type headerField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var event frameEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			fmt.Fprintf(os.Stderr, "skip invalid jsonl line: %v\n", err)
			continue
		}

		fmt.Printf("%s stream=%d %s\n", event.Direction, event.StreamID, event.Summary)
		for _, field := range event.DecodedHeaders {
			fmt.Printf("  header %s: %s\n", field.Name, field.Value)
		}
		if event.DataText != "" {
			fmt.Printf("  data %s\n", event.DataText)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
