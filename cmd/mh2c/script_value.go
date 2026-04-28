package main

import "fmt"

type scriptTable map[string]scriptValue

type scriptValue struct {
	kind    scriptValueKind
	str     string
	number  int64
	boolean bool
	list    []string
}

type scriptValueKind int

const (
	scriptString scriptValueKind = iota + 1
	scriptNumber
	scriptBool
	scriptStringList
)

func (t scriptTable) requireString(key string) (string, error) {
	value, ok, err := t.stringValue(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func (t scriptTable) stringValue(key string) (string, bool, error) {
	value, ok := t[key]
	if !ok {
		return "", false, nil
	}
	if value.kind != scriptString {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	return value.str, true, nil
}

func (t scriptTable) stringListValue(key string) ([]string, bool, error) {
	value, ok := t[key]
	if !ok {
		return nil, false, nil
	}
	if value.kind != scriptStringList {
		return nil, false, fmt.Errorf("%s must be an array of strings", key)
	}
	return append([]string(nil), value.list...), true, nil
}

func (t scriptTable) boolValue(key string) (bool, bool, error) {
	value, ok := t[key]
	if !ok {
		return false, false, nil
	}
	if value.kind != scriptBool {
		return false, false, fmt.Errorf("%s must be a bool", key)
	}
	return value.boolean, true, nil
}

func (t scriptTable) intValue(key string) (int64, bool, error) {
	value, ok := t[key]
	if !ok {
		return 0, false, nil
	}
	if value.kind != scriptNumber {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	return value.number, true, nil
}

func (t scriptTable) requireUint32(key string) (uint32, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	if value < 0 || value > int64(^uint32(0)) {
		return 0, fmt.Errorf("%s must fit in uint32", key)
	}
	return uint32(value), nil
}

func (t scriptTable) optionalUint32(key string) (uint32, bool, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if value < 0 || value > int64(^uint32(0)) {
		return 0, false, fmt.Errorf("%s must fit in uint32", key)
	}
	return uint32(value), true, nil
}

func (t scriptTable) requireUint8(key string) (uint8, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	if value < 0 || value > 0xff {
		return 0, fmt.Errorf("%s must fit in uint8", key)
	}
	return uint8(value), nil
}

func (t scriptTable) optionalUint8(key string) (uint8, bool, error) {
	value, ok, err := t.intValue(key)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	if value < 0 || value > 0xff {
		return 0, false, fmt.Errorf("%s must fit in uint8", key)
	}
	return uint8(value), true, nil
}
