package main

import (
	"fmt"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

func (o *outputController) writeTextFrame(prefix string, f frame.Frame, headers []hpack.HeaderField, warnings []string) error {
	if _, err := fmt.Fprintf(o.out, "%s %s\n", prefix, client.DebugFrameString(f)); err != nil {
		return err
	}

	switch typed := f.(type) {
	case frame.SettingsFrame:
		if len(typed.Settings) == 0 {
			if _, err := fmt.Fprintln(o.out, "  settings: <empty>"); err != nil {
				return err
			}
		} else {
			for _, setting := range typed.Settings {
				if _, err := fmt.Fprintf(o.out, "  setting id=%s value=%d\n", setting.ID, setting.Value); err != nil {
					return err
				}
			}
		}
	case frame.HeadersFrame:
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  header-block-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.ContinuationFrame:
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  continuation-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.PushPromiseFrame:
		if _, err := fmt.Fprintf(o.out, "  promised-stream-id: %d\n", typed.PromisedStreamID); err != nil {
			return err
		}
		if o.showHeaderBlock {
			if _, err := fmt.Fprintf(o.out, "  header-block-fragment: %s\n", formatHexSummary(typed.BlockFragment, o.dataLimit)); err != nil {
				return err
			}
		}
	case frame.DataFrame:
		if _, err := fmt.Fprintf(o.out, "  data-length: %d\n", len(typed.Data)); err != nil {
			return err
		}
		if err := o.writeTextPayload("data", typed.Data); err != nil {
			return err
		}
	case frame.PingFrame:
		if err := o.writeTextPayload("ping", typed.Data[:]); err != nil {
			return err
		}
	case frame.GoAwayFrame:
		if len(typed.DebugData) == 0 {
			if _, err := fmt.Fprintln(o.out, "  debug-data: <empty>"); err != nil {
				return err
			}
		} else if err := o.writeTextPayload("debug-data", typed.DebugData); err != nil {
			return err
		}
	case frame.RawFrame:
		if _, err := fmt.Fprintf(o.out, "  raw-payload-hex: %s\n", formatHexSummary(typed.Payload(), o.dataLimit)); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(o.out, "  payload-hex: %s\n", formatHexSummary(f.Payload(), o.dataLimit)); err != nil {
			return err
		}
	}

	for _, warning := range warnings {
		if _, err := fmt.Fprintf(o.out, "  header-warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, field := range headers {
		if _, err := fmt.Fprintf(o.out, "  header %s: %s\n", field.Name, field.Value); err != nil {
			return err
		}
	}
	return nil
}

func (o *outputController) writeTextPayload(label string, data []byte) error {
	switch o.dataFormat {
	case dataFormatText:
		_, err := fmt.Fprintf(o.out, "  %s-text: %s\n", label, formatDataTextLimited(data, o.dataLimit))
		return err
	case dataFormatHex:
		_, err := fmt.Fprintf(o.out, "  %s-hex: %s\n", label, formatHexSummary(data, o.dataLimit))
		return err
	default:
		if _, err := fmt.Fprintf(o.out, "  %s-hex: %s\n", label, formatHexSummary(data, o.dataLimit)); err != nil {
			return err
		}
		_, err := fmt.Fprintf(o.out, "  %s-text: %s\n", label, formatDataTextLimited(data, o.dataLimit))
		return err
	}
}
