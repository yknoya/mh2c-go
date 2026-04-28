package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yknoya/mh2c-go/frame"
)

func (o *outputController) shouldDisplayDirection(direction string) bool {
	if len(o.directionFilter) == 0 {
		return true
	}
	return o.directionFilter[direction]
}

func (o *outputController) shouldDisplay(direction string, f frame.Frame) bool {
	if !o.shouldDisplayDirection(direction) {
		return false
	}
	if len(o.frameFilters) > 0 && !o.frameFilters[frameTypeName(f)] {
		return false
	}
	if o.hasStreamFilter {
		streamID := f.Header().StreamID
		if streamID != 0 && streamID != o.streamFilter {
			return false
		}
	}
	return true
}

func buildFrameFilterSet(values []string) (map[string]bool, error) {
	if len(values) == 0 {
		return nil, nil
	}
	filters := make(map[string]bool, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if !isSupportedFrameFilter(name) {
			return nil, fmt.Errorf("invalid frame-filter %q", value)
		}
		filters[name] = true
	}
	return filters, nil
}

func buildDirectionFilterSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	filters := make(map[string]bool, len(values))
	for _, value := range values {
		filters[strings.ToLower(strings.TrimSpace(value))] = true
	}
	return filters
}

func validateDirectionFilters(values []string) error {
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if !isSupportedDirectionFilter(name) {
			return fmt.Errorf("invalid direction-filter %q", value)
		}
	}
	return nil
}

func isSupportedDirectionFilter(name string) bool {
	switch name {
	case "sent", "received":
		return true
	default:
		return false
	}
}

func isSupportedFrameFilter(name string) bool {
	switch name {
	case "data", "headers", "priority", "rst_stream", "settings", "push_promise", "ping", "goaway", "window_update", "continuation", "raw":
		return true
	default:
		return false
	}
}

func frameTypeName(f frame.Frame) string {
	switch f.(type) {
	case frame.DataFrame:
		return "data"
	case frame.HeadersFrame:
		return "headers"
	case frame.PriorityFrame:
		return "priority"
	case frame.RSTStreamFrame:
		return "rst_stream"
	case frame.SettingsFrame:
		return "settings"
	case frame.PushPromiseFrame:
		return "push_promise"
	case frame.PingFrame:
		return "ping"
	case frame.GoAwayFrame:
		return "goaway"
	case frame.WindowUpdateFrame:
		return "window_update"
	case frame.ContinuationFrame:
		return "continuation"
	case frame.RawFrame:
		return "raw"
	default:
		return strings.ToLower(strconv.FormatUint(uint64(f.Header().Type), 10))
	}
}
