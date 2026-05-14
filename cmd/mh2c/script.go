package main

import (
	"fmt"
	"time"

	"github.com/yknoya/mh2c-go/client"
)

type scriptFile struct {
	connection scriptTable
	actions    []scriptTable
}

func applyScriptConnection(cfg config, script scriptFile) (config, error) {
	if value, ok, err := script.connection.stringValue("url"); err != nil {
		return config{}, err
	} else if ok {
		cfg.rawURL = value
	}
	if value, ok, err := script.connection.stringValue("scheme"); err != nil {
		return config{}, err
	} else if ok {
		cfg.scheme = value
	}
	if value, ok, err := script.connection.stringValue("host"); err != nil {
		return config{}, err
	} else if ok {
		cfg.host = value
	}
	if value, ok, err := script.connection.stringValue("authority"); err != nil {
		return config{}, err
	} else if ok {
		cfg.authority = value
	}
	if value, ok, err := script.connection.stringValue("path"); err != nil {
		return config{}, err
	} else if ok {
		cfg.path = value
	}
	if value, ok, err := script.connection.intValue("port"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 || value > 65535 {
			return config{}, fmt.Errorf("connection.port %d is out of range", value)
		}
		cfg.port = uint(value)
	}
	if value, ok, err := script.connection.boolValue("insecure"); err != nil {
		return config{}, err
	} else if ok {
		cfg.insecure = value
	}
	if value, ok, err := script.connection.boolValue("send_goaway"); err != nil {
		return config{}, err
	} else if ok {
		cfg.sendGoAway = value
	}
	if value, ok, err := script.connection.intValue("timeout_ms"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 {
			return config{}, fmt.Errorf("connection.timeout_ms must be >= 0")
		}
		cfg.timeout = time.Duration(value) * time.Millisecond
	}
	if value, ok, err := script.connection.intValue("max_table_size"); err != nil {
		return config{}, err
	} else if ok {
		if value < 0 {
			return config{}, fmt.Errorf("connection.max_table_size must be >= 0")
		}
		cfg.maxTable = uint(value)
	}
	return cfg, nil
}

func executeScript(h2c *client.Client, script scriptFile, out *outputController) (bool, error) {
	sawGoAway := false

	for index, action := range script.actions {
		repeat, err := parseActionRepeat(action)
		if err != nil {
			return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
		}
		for iteration := int64(0); iteration < repeat.count; iteration++ {
			iterAction, err := actionForRepeatIteration(action, repeat, iteration)
			if err != nil {
				return sawGoAway, formatScriptActionError(index, repeat.count, iteration, err)
			}
			gotGoAway, err := executeScriptAction(h2c, iterAction, out)
			if err != nil {
				return sawGoAway, formatScriptActionError(index, repeat.count, iteration, err)
			}
			sawGoAway = sawGoAway || gotGoAway
		}
	}

	return sawGoAway, nil
}

type actionRepeat struct {
	count           int64
	streamIDStep    uint32
	hasStreamIDStep bool
}

func parseActionRepeat(action scriptTable) (actionRepeat, error) {
	repeat := actionRepeat{count: 1}
	_, hasRepeat := action["repeat"]
	if count, ok, err := action.intValue("repeat"); err != nil {
		return actionRepeat{}, err
	} else if ok {
		if count <= 0 {
			return actionRepeat{}, fmt.Errorf("repeat must be > 0")
		}
		repeat.count = count
	}
	if step, ok, err := action.optionalUint32("stream_id_step"); err != nil {
		return actionRepeat{}, err
	} else if ok {
		if !hasRepeat {
			return actionRepeat{}, fmt.Errorf("stream_id_step requires repeat")
		}
		repeat.streamIDStep = step
		repeat.hasStreamIDStep = true
	}
	return repeat, nil
}

func actionForRepeatIteration(action scriptTable, repeat actionRepeat, iteration int64) (scriptTable, error) {
	if !repeat.hasStreamIDStep {
		return action, nil
	}
	baseStreamID, err := action.requireUint32("stream_id")
	if err != nil {
		return nil, err
	}
	streamID := uint64(baseStreamID) + uint64(repeat.streamIDStep)*uint64(iteration)
	if streamID > uint64(^uint32(0)) {
		return nil, fmt.Errorf("stream_id overflows uint32")
	}
	out := make(scriptTable, len(action))
	for key, value := range action {
		out[key] = value
	}
	out["stream_id"] = scriptValue{kind: scriptNumber, number: int64(streamID)}
	return out, nil
}

func formatScriptActionError(index int, repeatCount int64, iteration int64, err error) error {
	if repeatCount == 1 {
		return fmt.Errorf("action %d: %w", index+1, err)
	}
	return fmt.Errorf("action %d iteration %d: %w", index+1, iteration+1, err)
}

func executeScriptAction(h2c *client.Client, action scriptTable, out *outputController) (bool, error) {
	actionType, err := action.requireString("type")
	if err != nil {
		return false, err
	}

	switch actionType {
	case "preface":
		if err := h2c.SendConnectionPreface(); err != nil {
			return false, err
		}
		if err := out.PrintNotice("sent", "preface", "CONNECTION_PREFACE"); err != nil {
			return false, err
		}
	case "sleep":
		duration, err := parseSleepDuration(action)
		if err != nil {
			return false, err
		}
		if err := out.PrintNotice("sent", "sleep", fmt.Sprintf("SLEEP %s", duration)); err != nil {
			return false, err
		}
		time.Sleep(duration)
	case "receive":
		return executeReceiveAction(h2c, action, out)
	default:
		sent, err := buildScriptFrame(h2c, action)
		if err != nil {
			return false, err
		}
		event, err := h2c.SendFrame(sent)
		if err != nil {
			return false, err
		}
		if err := out.HandleSent(event); err != nil {
			return false, err
		}
	}
	return false, nil
}

func parseSleepDuration(action scriptTable) (time.Duration, error) {
	durationMS, ok, err := action.intValue("duration_ms")
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("duration_ms is required")
	}
	if durationMS <= 0 {
		return 0, fmt.Errorf("duration_ms must be > 0")
	}
	return time.Duration(durationMS) * time.Millisecond, nil
}
