package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

type CharmWriter struct {
	clog *log.Logger
}

func NewCharmWriter() *CharmWriter {
	styles := log.DefaultStyles()
	// Customize colors to match BRAND GUIDELINES
	// Brand Orange: #FFA60C, Green: #22C55E, Error: #FF3333
	
	clog := log.NewWithOptions(os.Stdout, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.TimeOnly,
		Prefix:          "StacyVM",
	})
	clog.SetStyles(styles)
	
	return &CharmWriter{
		clog: clog,
	}
}

func (w *CharmWriter) Write(p []byte) (n int, err error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(p, &fields); err != nil {
		w.clog.Print(string(p))
		return len(p), nil
	}

	msg := ""
	if m, ok := fields["message"].(string); ok {
		msg = m
	} else if errStr, ok := fields["error"].(string); ok {
		msg = errStr
	}
	
	lvl := "info"
	if l, ok := fields["level"].(string); ok {
		lvl = l
	}

	delete(fields, "message")
	delete(fields, "level")
	delete(fields, "time") // charm handles its own timestamp

	var args []interface{}
	for k, v := range fields {
		args = append(args, k, v)
	}

	switch lvl {
	case "info":
		w.clog.Info(msg, args...)
	case "error", "fatal", "panic":
		w.clog.Error(msg, args...)
	case "warn":
		w.clog.Warn(msg, args...)
	case "debug":
		w.clog.Debug(msg, args...)
	default:
		w.clog.Print(msg, args...)
	}

	return len(p), nil
}
