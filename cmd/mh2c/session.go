package main

import (
	"fmt"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
)

func startSession(h2c *client.Client, maxTable uint32, out *outputController) error {
	if err := h2c.SendConnectionPreface(); err != nil {
		return err
	}
	if err := out.PrintNotice("sent", "preface", "CONNECTION_PREFACE"); err != nil {
		return err
	}
	settings := frame.NewSettingsFrame(0, []frame.Setting{
		{ID: frame.SettingEnablePush, Value: 0},
		{ID: frame.SettingInitialWindowSize, Value: 65535},
		{ID: frame.SettingHeaderTableSize, Value: maxTable},
	})
	if err := sendFrameAndReport(h2c, out, settings); err != nil {
		return err
	}

	for {
		event, err := h2c.ReceiveFrame()
		if err != nil {
			return err
		}
		if err := out.HandleReceived(event); err != nil {
			return err
		}

		switch typed := event.Frame.(type) {
		case frame.SettingsFrame:
			if typed.Header().Flags&frame.FlagSettingsAck != 0 {
				continue
			}
			ack := frame.NewSettingsFrame(frame.FlagSettingsAck, nil)
			return sendFrameAndReport(h2c, out, ack)
		case frame.PingFrame:
			if typed.Header().Flags&frame.FlagPingAck == 0 {
				ack := frame.NewPingFrame(frame.FlagPingAck, typed.Data)
				if err := sendFrameAndReport(h2c, out, ack); err != nil {
					return err
				}
			}
		case frame.GoAwayFrame:
			return nil
		}
	}
}

func sendFrameAndReport(h2c *client.Client, out *outputController, f frame.Frame) error {
	event, err := h2c.SendFrame(f)
	if err != nil {
		return err
	}
	return out.HandleSent(event)
}

func ackControlFrame(h2c *client.Client, out *outputController, f frame.Frame) error {
	switch typed := f.(type) {
	case frame.SettingsFrame:
		if typed.Header().Flags&frame.FlagSettingsAck == 0 {
			return sendFrameAndReport(h2c, out, frame.NewSettingsFrame(frame.FlagSettingsAck, nil))
		}
	case frame.PingFrame:
		if typed.Header().Flags&frame.FlagPingAck == 0 {
			return sendFrameAndReport(h2c, out, frame.NewPingFrame(frame.FlagPingAck, typed.Data))
		}
	}
	return nil
}

func receiveResponseFrames(h2c *client.Client, streamID uint32, out *outputController) (bool, error) {
	sawGoAway := false

	for {
		event, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		if err := out.HandleReceived(event); err != nil {
			return sawGoAway, err
		}
		if err := ackControlFrame(h2c, out, event.Frame); err != nil {
			return sawGoAway, err
		}

		if _, ok := event.Frame.(frame.GoAwayFrame); ok {
			sawGoAway = true
			return sawGoAway, nil
		}

		if event.DecodeError != nil {
			return sawGoAway, event.DecodeError
		}
		if event.HeaderBlockComplete && event.HeaderBlockStreamID == streamID && event.HeaderBlockEndStream {
			return sawGoAway, nil
		}
		if typed, ok := event.Frame.(frame.DataFrame); ok && typed.Header().StreamID == streamID && typed.Header().Flags&frame.FlagDataEndStream != 0 {
			return sawGoAway, nil
		}
	}
}

func receivePingFrames(h2c *client.Client, want [8]byte, out *outputController) (bool, error) {
	sawGoAway := false
	for {
		event, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		if err := out.HandleReceived(event); err != nil {
			return sawGoAway, err
		}
		if err := ackControlFrame(h2c, out, event.Frame); err != nil {
			return sawGoAway, err
		}

		switch typed := event.Frame.(type) {
		case frame.PingFrame:
			if typed.Header().Flags&frame.FlagPingAck == 0 {
				continue
			}
			if typed.Data == want {
				return sawGoAway, nil
			}
		case frame.GoAwayFrame:
			sawGoAway = true
			return sawGoAway, nil
		}
	}
}

func receiveObserveFrames(h2c *client.Client, maxRecv uint, out *outputController) (bool, error) {
	sawGoAway := false
	var received uint

	for {
		if maxRecv > 0 && received >= maxRecv {
			return sawGoAway, nil
		}
		event, err := h2c.ReceiveFrame()
		if err != nil {
			return sawGoAway, err
		}
		received++
		if err := out.HandleReceived(event); err != nil {
			return sawGoAway, err
		}
		if err := ackControlFrame(h2c, out, event.Frame); err != nil {
			return sawGoAway, err
		}

		if _, ok := event.Frame.(frame.GoAwayFrame); ok {
			sawGoAway = true
			return sawGoAway, nil
		}
	}
}

func parsePingData(src string) ([8]byte, error) {
	var payload [8]byte
	if len(src) != len(payload) {
		return payload, fmt.Errorf("ping-data must be exactly 8 bytes, got %d", len(src))
	}
	copy(payload[:], src)
	return payload, nil
}
