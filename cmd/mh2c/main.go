package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yknoya/mh2c-go/client"
	"github.com/yknoya/mh2c-go/frame"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	switch cfg.scriptCommand {
	case "describe":
		return describeScriptActions(stdout, cfg.scriptDescribe)
	case "template":
		return writeScriptTemplate(stdout, cfg.scriptTemplate)
	case "validate":
		script, err := parseScriptFile(cfg.scriptFile)
		if err != nil {
			return err
		}
		return validateScript(script)
	}
	var script scriptFile
	if cfg.mode == "script" {
		script, err = parseScriptFile(cfg.scriptFile)
		if err != nil {
			return err
		}
		if err := validateScript(script); err != nil {
			return err
		}
		cfg, err = applyScriptConnection(cfg, script)
		if err != nil {
			return err
		}
	}
	ep, err := resolveEndpoint(cfg)
	if err != nil {
		return err
	}
	out, closeOutput, err := prepareOutputWriter(stdout, cfg.saveOutput)
	if err != nil {
		return err
	}
	defer closeOutput()
	controller, err := newOutputController(out, cfg)
	if err != nil {
		return err
	}

	ctx := parent
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, cfg.timeout)
		defer cancel()
	}

	opts := []client.Option{client.WithMaxDynamicTableSize(uint32(cfg.maxTable))}
	if cfg.insecure {
		opts = append(opts, client.WithInsecureSkipVerify())
	}

	h2c, err := client.New(ctx, ep.host, ep.port, opts...)
	if err != nil {
		return err
	}
	defer h2c.Close()

	streamID := uint32(cfg.streamID)
	sawGoAway := false
	switch cfg.mode {
	case "request":
		if err := startSession(h2c, uint32(cfg.maxTable), controller); err != nil {
			return err
		}
		body, err := readRequestBody(cfg, stdin)
		if err != nil {
			return err
		}
		fields, err := buildRequestFields(ep, cfg, body)
		if err != nil {
			return err
		}
		if err := sendRequest(h2c, streamID, fields, body, controller); err != nil {
			return err
		}
		sawGoAway, err = receiveResponseFrames(h2c, streamID, controller)
		if err != nil {
			return err
		}
	case "ping":
		if err := startSession(h2c, uint32(cfg.maxTable), controller); err != nil {
			return err
		}
		payload, err := parsePingData(cfg.pingData)
		if err != nil {
			return err
		}
		pingFrame := frame.PingFrame{Data: payload}
		if err := sendFrameAndReport(h2c, controller, pingFrame); err != nil {
			return err
		}
		sawGoAway, err = receivePingFrames(h2c, payload, controller)
		if err != nil {
			return err
		}
	case "observe":
		if err := startSession(h2c, uint32(cfg.maxTable), controller); err != nil {
			return err
		}
		sawGoAway, err = receiveObserveFrames(h2c, cfg.maxRecv, controller)
		if err != nil {
			return err
		}
	case "script":
		sawGoAway, err = executeScript(h2c, script, controller)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}

	if cfg.sendGoAway && !sawGoAway {
		goAway := frame.GoAwayFrame{
			LastStreamID: streamID,
			ErrorCode:    frame.ErrNo,
		}
		if err := h2c.SendFrame(goAway); err == nil {
			_ = controller.HandleSent(h2c, goAway)
		}
	}
	return controller.Flush()
}
