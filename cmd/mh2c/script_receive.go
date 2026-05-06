package main

import (
	"fmt"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
)

func executeReceiveAction(h2c *client.Client, action scriptTable, out *outputController) (bool, error) {
	if _, ok := action["count"]; ok {
		if _, ok := action["until"]; ok {
			return false, fmt.Errorf("count and until cannot be used together")
		}
	}

	count := int64(1)
	if value, ok, err := action.intValue("count"); err != nil {
		return false, err
	} else if ok {
		if value <= 0 {
			return false, fmt.Errorf("count must be > 0")
		}
		count = value
	}
	until, _, err := action.stringValue("until")
	if err != nil {
		return false, err
	}
	streamID, hasStreamID, err := action.optionalUint32("stream_id")
	if err != nil {
		return false, err
	}
	ackSettings, _, err := action.boolValue("ack_settings")
	if err != nil {
		return false, err
	}
	ackPing, _, err := action.boolValue("ack_ping")
	if err != nil {
		return false, err
	}
	if until == "end_stream" && !hasStreamID {
		return false, fmt.Errorf("stream_id is required when until=end_stream")
	}

	var (
		receivedCount int64
		sawGoAway     bool
		pendingStream uint32
		pendingBlock  []byte
		pendingEnd    bool
	)

	for {
		received, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		receivedCount++
		if err := out.HandleReceived(h2c, received); err != nil {
			return sawGoAway, err
		}
		if headers, _, stream, endStream, err := consumeHeaderBlockForDisplay(&pendingStream, &pendingBlock, &pendingEnd, received, h2c.DecodeHeadersDetailed); err != nil {
			return sawGoAway, err
		} else if len(headers) > 0 && until == "end_stream" && hasStreamID && stream == streamID && endStream {
			return sawGoAway, nil
		}

		switch typed := received.(type) {
		case frame.SettingsFrame:
			if ackSettings && typed.Header().Flags&frame.FlagSettingsAck == 0 {
				ack := frame.NewSettingsFrame(frame.FlagSettingsAck, nil)
				if err := h2c.SendFrame(ack); err != nil {
					return sawGoAway, err
				}
				if err := out.HandleSent(h2c, ack); err != nil {
					return sawGoAway, err
				}
			}
			if until == "settings" && typed.Header().Flags&frame.FlagSettingsAck == 0 {
				return sawGoAway, nil
			}
			if until == "settings_ack" && typed.Header().Flags&frame.FlagSettingsAck != 0 {
				return sawGoAway, nil
			}
		case frame.PingFrame:
			if ackPing && typed.Header().Flags&frame.FlagPingAck == 0 {
				ack := frame.NewPingFrame(frame.FlagPingAck, typed.Data)
				if err := h2c.SendFrame(ack); err != nil {
					return sawGoAway, err
				}
				if err := out.HandleSent(h2c, ack); err != nil {
					return sawGoAway, err
				}
			}
			if until == "ping_ack" && typed.Header().Flags&frame.FlagPingAck != 0 {
				return sawGoAway, nil
			}
		case frame.GoAwayFrame:
			sawGoAway = true
			if until == "" || until == "goaway" {
				return sawGoAway, nil
			}
		case frame.DataFrame:
			if until == "end_stream" && hasStreamID && typed.Header().StreamID == streamID && typed.Header().Flags&frame.FlagDataEndStream != 0 {
				return sawGoAway, nil
			}
		case frame.HeadersFrame:
			if until == "end_stream" && hasStreamID && typed.Header().StreamID == streamID && typed.Header().Flags&frame.FlagHeadersEndStream != 0 {
				return sawGoAway, nil
			}
		}

		if until == "" && receivedCount >= count {
			return sawGoAway, nil
		}
	}
}
