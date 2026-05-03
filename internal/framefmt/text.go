package framefmt

import (
	"fmt"
	"io"

	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
)

type TextFrame struct {
	Prefix          string
	Frame           frame.Frame
	Headers         []hpack.HeaderField
	Warnings        []string
	ShowHeaderBlock bool
	DataFormat      string
	DataLimit       uint
}

func Summary(f frame.Frame) string {
	switch typed := f.(type) {
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%T len=%d", f, len(f.Payload()))
	}
}

func WriteTextFrame(w io.Writer, opts TextFrame) error {
	if _, err := fmt.Fprintf(w, "%s %s\n", opts.Prefix, Summary(opts.Frame)); err != nil {
		return err
	}

	switch typed := opts.Frame.(type) {
	case frame.SettingsFrame:
		if len(typed.Settings) == 0 {
			if _, err := fmt.Fprintln(w, "  settings: <empty>"); err != nil {
				return err
			}
		} else {
			for _, setting := range typed.Settings {
				if _, err := fmt.Fprintf(w, "  setting id=%s value=%d\n", setting.ID, setting.Value); err != nil {
					return err
				}
			}
		}
	case frame.HeadersFrame:
		if opts.ShowHeaderBlock {
			if _, err := fmt.Fprintf(w, "  header-block-fragment: %s\n", HexSummary(typed.BlockFragment, opts.DataLimit)); err != nil {
				return err
			}
		}
	case frame.ContinuationFrame:
		if opts.ShowHeaderBlock {
			if _, err := fmt.Fprintf(w, "  continuation-fragment: %s\n", HexSummary(typed.BlockFragment, opts.DataLimit)); err != nil {
				return err
			}
		}
	case frame.PushPromiseFrame:
		if _, err := fmt.Fprintf(w, "  promised-stream-id: %d\n", typed.PromisedStreamID); err != nil {
			return err
		}
		if opts.ShowHeaderBlock {
			if _, err := fmt.Fprintf(w, "  header-block-fragment: %s\n", HexSummary(typed.BlockFragment, opts.DataLimit)); err != nil {
				return err
			}
		}
	case frame.DataFrame:
		if _, err := fmt.Fprintf(w, "  data-length: %d\n", len(typed.Data)); err != nil {
			return err
		}
		if err := writeTextPayload(w, opts.DataFormat, opts.DataLimit, "data", typed.Data); err != nil {
			return err
		}
	case frame.PingFrame:
		if err := writeTextPayload(w, opts.DataFormat, opts.DataLimit, "ping", typed.Data[:]); err != nil {
			return err
		}
	case frame.GoAwayFrame:
		if len(typed.DebugData) == 0 {
			if _, err := fmt.Fprintln(w, "  debug-data: <empty>"); err != nil {
				return err
			}
		} else if err := writeTextPayload(w, opts.DataFormat, opts.DataLimit, "debug-data", typed.DebugData); err != nil {
			return err
		}
	case frame.RawFrame:
		if _, err := fmt.Fprintf(w, "  raw-payload-hex: %s\n", HexSummary(typed.Payload(), opts.DataLimit)); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(w, "  payload-hex: %s\n", HexSummary(opts.Frame.Payload(), opts.DataLimit)); err != nil {
			return err
		}
	}

	for _, warning := range opts.Warnings {
		if _, err := fmt.Fprintf(w, "  header-warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, field := range opts.Headers {
		if _, err := fmt.Fprintf(w, "  header %s: %s\n", field.Name, field.Value); err != nil {
			return err
		}
	}
	return nil
}

func writeTextPayload(w io.Writer, dataFormat string, limit uint, label string, data []byte) error {
	switch dataFormat {
	case DataFormatText:
		_, err := fmt.Fprintf(w, "  %s-text: %s\n", label, DataTextLimited(data, limit))
		return err
	case DataFormatHex:
		_, err := fmt.Fprintf(w, "  %s-hex: %s\n", label, HexSummary(data, limit))
		return err
	default:
		if _, err := fmt.Fprintf(w, "  %s-hex: %s\n", label, HexSummary(data, limit)); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "  %s-text: %s\n", label, DataTextLimited(data, limit))
		return err
	}
}
