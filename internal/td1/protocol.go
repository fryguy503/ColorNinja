// Package td1 implements the TD1/TD1S USB protocol. See docs/td1-research.md
// for wire-level references. No vendor implementation or firmware is bundled.
package td1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type Reading struct {
	ID         uint64  `json:"id"`
	TD         float64 `json:"td"`
	Color      string  `json:"color"`
	Serial     string  `json:"serial"`
	CapturedAt string  `json:"capturedAt"`
	Raw        string  `json:"raw"`
}

func ParseDisplay(line string) (DisplayLine, bool, bool) {
	if line == "clearScreen" {
		return DisplayLine{}, true, true
	}
	p := strings.Split(line, ",")
	if len(p) < 2 || strings.TrimSpace(p[0]) != "display" {
		return DisplayLine{}, false, false
	}
	if strings.TrimSpace(p[1]) == "ClearScreen" {
		return DisplayLine{}, true, true
	}
	if len(p) < 4 {
		return DisplayLine{}, false, false
	}
	x, e1 := strconv.Atoi(strings.TrimSpace(p[len(p)-2]))
	y, e2 := strconv.Atoi(strings.TrimSpace(p[len(p)-1]))
	if e1 != nil || e2 != nil || x < 0 || x > 255 || y < 0 || y > 127 {
		return DisplayLine{}, false, false
	}
	return DisplayLine{Text: strings.TrimSpace(strings.Join(p[1:len(p)-2], ",")), X: x, Y: y}, false, true
}
func ParseReading(line string) (Reading, bool) {
	p := strings.Split(strings.TrimSpace(line), ",")
	if len(p) != 6 && len(p) != 9 {
		return Reading{}, false
	}
	if strings.EqualFold(strings.TrimSpace(p[0]), "display") {
		return Reading{}, false
	}
	td, err := strconv.ParseFloat(strings.TrimSpace(p[4]), 64)
	color := strings.TrimSpace(p[5])
	color = strings.TrimPrefix(color, "#")
	if err != nil || math.IsNaN(td) || math.IsInf(td, 0) || td <= 0 || td > 1000 || len(color) != 6 {
		return Reading{}, false
	}
	if _, err = strconv.ParseUint(color, 16, 24); err != nil {
		return Reading{}, false
	}
	// Firmware calibration records deliberately leave sensor fields 1–3 empty.
	// The vendor's Moonraker integration identifies TD/color by fields 4 and 5.
	return Reading{TD: td, Color: "#" + strings.ToUpper(color), CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Raw: line}, true
}

type wire struct {
	port    io.ReadWriter
	pending []byte
	observe func(string)
}

func (w *wire) send(data []byte) error {
	for len(data) > 0 {
		n, err := w.port.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
func (w *wire) fill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	n, err := w.port.Read(buf)
	if n > 0 {
		w.pending = append(w.pending, buf[:n]...)
	}
	if len(w.pending) > 1<<20 {
		return fmt.Errorf("TD1 response exceeds size limit")
	}
	if n > 0 {
		return nil
	}
	return err
}
func (w *wire) line(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if at := bytes.IndexByte(w.pending, '\n'); at >= 0 {
			line := strings.TrimSpace(string(w.pending[:at]))
			w.pending = w.pending[at+1:]
			if w.observe != nil {
				w.observe(line)
			}
			return line, nil
		}
		if err := w.fill(ctx); err != nil {
			return "", err
		}
	}
}
func (w *wire) exact(ctx context.Context, n int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for len(w.pending) < n {
		if err := w.fill(ctx); err != nil {
			return nil, err
		}
	}
	raw := append([]byte(nil), w.pending[:n]...)
	w.pending = w.pending[n:]
	return raw, nil
}
func (w *wire) reply(ctx context.Context) (string, error) {
	for {
		line, err := w.line(ctx)
		if err != nil {
			return "", err
		}
		if line == "" || strings.HasPrefix(line, "display,") || line == "clear" || line == "clearScreen" {
			continue
		}
		if _, ok := ParseReading(line); ok {
			continue
		}
		return line, nil
	}
}
func (w *wire) ready(ctx context.Context) error {
	line, err := w.reply(ctx)
	if err != nil {
		return err
	}
	if line != "ready" {
		return fmt.Errorf("TD1 expected ready: %s", line)
	}
	return nil
}
func (w *wire) command(ctx context.Context, command string) (string, error) {
	if err := w.send([]byte(command + "\n")); err != nil {
		return "", err
	}
	return w.reply(ctx)
}
func (w *wire) data(ctx context.Context, command, data string) (string, error) {
	if err := w.send([]byte(command + "\n")); err != nil {
		return "", err
	}
	if err := w.ready(ctx); err != nil {
		return "", err
	}
	if err := w.send([]byte(data)); err != nil {
		return "", err
	}
	return w.reply(ctx)
}
func (w *wire) number(ctx context.Context, max int) (int, error) {
	line, err := w.line(ctx)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 0 || n > max {
		return 0, fmt.Errorf("invalid TD1 transfer size %q", line)
	}
	return n, nil
}
func (w *wire) receive(ctx context.Context, name string) ([]byte, error) {
	if err := w.send([]byte("retrieve file\n" + name + "\n")); err != nil {
		return nil, err
	}
	line, err := w.reply(ctx)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(line, "No file named ") {
		return nil, fmt.Errorf("%w: %s", ErrNoFile, name)
	}
	if line != "ready" {
		return nil, fmt.Errorf("TD1 could not retrieve %s: %s", name, line)
	}
	size, err := w.number(ctx, 1<<20)
	if err != nil {
		return nil, err
	}
	blocks, err := w.number(ctx, 4096)
	if err != nil {
		return nil, err
	}
	var result []byte
	for i := 0; i < blocks; i++ {
		n, err := w.number(ctx, 1<<20)
		if err != nil {
			return nil, err
		}
		if n == 0 || len(result)+n > size {
			return nil, fmt.Errorf("invalid TD1 block length")
		}
		raw, err := w.exact(ctx, n)
		if err != nil {
			return nil, err
		}
		result = append(result, raw...)
		if err = w.send([]byte("ready\n")); err != nil {
			return nil, err
		}
	}
	if len(result) != size {
		return nil, fmt.Errorf("incomplete TD1 file: received %d of %d bytes", len(result), size)
	}
	return result, nil
}
func (w *wire) sendFile(ctx context.Context, name string, raw []byte) error {
	if err := w.send([]byte("file\n" + name + "\n")); err != nil {
		return err
	}
	if err := w.ready(ctx); err != nil {
		return err
	}
	if err := w.send([]byte(fmt.Sprintf("%d\n%d\n", len(raw), (len(raw)+1023)/1024))); err != nil {
		return err
	}
	for at := 0; at < len(raw); at += 1024 {
		end := min(at+1024, len(raw))
		if err := w.send(append([]byte(fmt.Sprintf("%d\n", end-at)), raw[at:end]...)); err != nil {
			return err
		}
		if err := w.ready(ctx); err != nil {
			return err
		}
	}
	return nil
}
