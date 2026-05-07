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
		actionType, err := action.requireString("type")
		if err != nil {
			return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
		}

		switch actionType {
		case "preface":
			if err := h2c.SendConnectionPreface(); err != nil {
				return sawGoAway, err
			}
			if err := out.PrintNotice("sent", "preface", "CONNECTION_PREFACE"); err != nil {
				return sawGoAway, err
			}
		case "sleep":
			duration, err := parseSleepDuration(action)
			if err != nil {
				return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
			}
			if err := out.PrintNotice("sent", "sleep", fmt.Sprintf("SLEEP %s", duration)); err != nil {
				return sawGoAway, err
			}
			time.Sleep(duration)
		case "receive":
			gotGoAway, err := executeReceiveAction(h2c, action, out)
			if err != nil {
				return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
			}
			sawGoAway = sawGoAway || gotGoAway
		default:
			sent, err := buildScriptFrame(h2c, action)
			if err != nil {
				return sawGoAway, fmt.Errorf("action %d: %w", index+1, err)
			}
			event, err := h2c.SendFrame(sent)
			if err != nil {
				return sawGoAway, err
			}
			if err := out.HandleSent(event); err != nil {
				return sawGoAway, err
			}
		}
	}

	return sawGoAway, nil
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
