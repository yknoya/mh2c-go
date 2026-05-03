package main

import (
	"github.com/yknoya/mh2c-go/frame"
	"github.com/yknoya/mh2c-go/hpack"
	"github.com/yknoya/mh2c-go/internal/framefmt"
)

func (o *outputController) writeTextFrame(prefix string, f frame.Frame, headers []hpack.HeaderField, warnings []string) error {
	return framefmt.WriteTextFrame(o.out, framefmt.TextFrame{
		Prefix:          prefix,
		Frame:           f,
		Headers:         headers,
		Warnings:        warnings,
		ShowHeaderBlock: o.showHeaderBlock,
		DataFormat:      o.dataFormat,
		DataLimit:       o.dataLimit,
	})
}
