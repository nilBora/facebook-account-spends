package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"

	"github.com/fatih/color"
)

type handler struct {
	mu     sync.Mutex
	out    io.Writer
	level  slog.Level
	attrs  []slog.Attr
	debug  bool // colored output + caller info
}

// NewHandler returns a handler that writes:
//
//	2006-01-02 15:04:05 [INFO] message key=value ...
func NewHandler(out io.Writer, level slog.Level) slog.Handler {
	return &handler{out: out, level: level}
}

// NewDebugHandler returns a handler with colored output and caller info for debug mode.
func NewDebugHandler(out io.Writer) slog.Handler {
	return &handler{out: out, level: slog.LevelDebug, debug: true}
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	timeStr := r.Time.Format("2006/01/02 15:04:05.000")
	levelStr := "[" + r.Level.String() + "]"

	if h.debug {
		timeStr = color.New(color.FgCyan).Sprint(timeStr)
		switch r.Level {
		case slog.LevelError:
			levelStr = color.New(color.FgHiRed).Sprint(levelStr)
		case slog.LevelWarn:
			levelStr = color.New(color.FgHiYellow).Sprint(levelStr)
		case slog.LevelInfo:
			levelStr = color.New(color.FgHiWhite).Sprint(levelStr)
		default: // DEBUG
			levelStr = color.New(color.FgWhite).Sprint(levelStr)
		}
	} else {
		timeStr = r.Time.Format("2006/01/02 15:04:05.000")
	}

	buf.WriteString(timeStr)
	buf.WriteByte(' ')
	buf.WriteString(levelStr)

	if h.debug && r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		file := frame.File
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			file = file[idx+1:]
		}
		caller := fmt.Sprintf(" %s:%d", file, frame.Line)
		buf.WriteString(color.New(color.FgBlue).Sprint(caller))
	}

	buf.WriteByte(' ')
	buf.WriteString(r.Message)

	for _, a := range h.attrs {
		writeAttr(&buf, a, h.debug)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&buf, a, h.debug)
		return true
	})

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf.Bytes())
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &handler{out: h.out, level: h.level, attrs: merged, debug: h.debug}
}

func (h *handler) WithGroup(_ string) slog.Handler { return h }

func writeAttr(buf *bytes.Buffer, a slog.Attr, clr bool) {
	v := a.Value.Resolve()
	kv := a.Key + "="
	if v.Kind() == slog.KindString {
		s := v.String()
		if containsSpace(s) {
			kv += fmt.Sprintf("%q", s)
		} else {
			kv += s
		}
	} else {
		kv += fmt.Sprintf("%v", v.Any())
	}

	buf.WriteByte(' ')
	if clr {
		buf.WriteString(color.New(color.FgHiBlack).Sprint(kv))
	} else {
		buf.WriteString(kv)
	}
}

func containsSpace(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' {
			return true
		}
	}
	return false
}
