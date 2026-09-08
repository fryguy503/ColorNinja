package engine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AtomicWrite creates a complete sibling temporary file before publishing it.
// Hard-link creation implements race-safe no-clobber publication on NTFS.
func AtomicWrite(path string, overwrite bool, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".colorninja-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = write(f); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if overwrite {
		return os.Rename(tmp, path)
	}
	if e = os.Link(tmp, path); e != nil {
		if _, se := os.Stat(path); se == nil {
			return fmt.Errorf("output already exists: %s", path)
		}
		return fmt.Errorf("publish output without overwrite: %w", e)
	}
	return nil
}

type checkedWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w checkedWriter) Write(p []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	return w.w.Write(p)
}

type metadataWriter struct {
	w        io.Writer
	prefix   []byte
	dpi      [2]float64
	inserted bool
}

func writeChunk(w io.Writer, kind string, data []byte) error {
	b := make([]byte, 12+len(data))
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], kind)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
	_, e := w.Write(b)
	return e
}
func (m *metadataWriter) Write(p []byte) (int, error) {
	n := len(p)
	if !m.inserted {
		take := min(33-len(m.prefix), len(p))
		m.prefix = append(m.prefix, p[:take]...)
		p = p[take:]
		if len(m.prefix) < 33 {
			return n, nil
		}
		if _, e := m.w.Write(m.prefix); e != nil {
			return 0, e
		}
		if e := writeChunk(m.w, "sRGB", []byte{0}); e != nil {
			return 0, e
		}
		if m.dpi[0] > 0 && m.dpi[1] > 0 {
			d := make([]byte, 9)
			binary.BigEndian.PutUint32(d, uint32(m.dpi[0]/.0254+.5))
			binary.BigEndian.PutUint32(d[4:], uint32(m.dpi[1]/.0254+.5))
			d[8] = 1
			if e := writeChunk(m.w, "pHYs", d); e != nil {
				return 0, e
			}
		}
		m.inserted = true
	}
	if len(p) > 0 {
		if _, e := m.w.Write(p); e != nil {
			return 0, e
		}
	}
	return n, nil
}
func WritePNG(ctx context.Context, w io.Writer, img image.Image, dpi [2]float64) error {
	writer := &metadataWriter{w: checkedWriter{ctx, w}, dpi: dpi}
	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	return encoder.Encode(writer, img)
}
func SavePNG(ctx context.Context, path string, r *Result, meta ImageMetadata, overwrite bool) error {
	if strings.ToLower(filepath.Ext(path)) != ".png" {
		return fmt.Errorf("output filename must end in .png")
	}
	return AtomicWrite(path, overwrite, func(w io.Writer) error { return WritePNG(ctx, w, r.Image, meta.DPI) })
}
func SaveLayerMap(ctx context.Context, path string, r *Result, overwrite bool) error {
	if r.Stack == nil || len(r.LayerMap) == 0 {
		return fmt.Errorf("layer maps require stack planning mode")
	}
	if strings.ToLower(filepath.Ext(path)) != ".png" {
		return fmt.Errorf("layer map filename must end in .png")
	}
	w, h := r.SourceSize[0], r.SourceSize[1]
	img := image.NewGray16(image.Rect(0, 0, w, h))
	for i, v := range r.LayerMap {
		binary.BigEndian.PutUint16(img.Pix[i*2:], v)
	}
	return AtomicWrite(path, overwrite, func(w io.Writer) error { return png.Encode(checkedWriter{ctx, w}, img) })
}

type Report struct {
	SchemaVersion int           `json:"schemaVersion"`
	Engine        string        `json:"engine"`
	Mode          string        `json:"mode"`
	Source        string        `json:"source"`
	Options       Options       `json:"options"`
	Metadata      ImageMetadata `json:"metadata"`
	Result        *Result       `json:"result"`
}

func SaveReport(path, source string, r *Result, o Options, meta ImageMetadata, overwrite bool) error {
	return AtomicWrite(path, overwrite, func(w io.Writer) error {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		return e.Encode(Report{1, "ColorNinja Go", o.Mode, filepath.Base(source), o, meta, r})
	})
}
func DistinctPaths(paths ...string) error {
	for i, p := range paths {
		if p == "" {
			continue
		}
		a, e := filepath.Abs(p)
		if e != nil {
			return e
		}
		sa, ea := os.Stat(p)
		for _, q := range paths[:i] {
			if q == "" {
				continue
			}
			b, e := filepath.Abs(q)
			if e != nil {
				return e
			}
			sb, eb := os.Stat(q)
			if strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) || (ea == nil && eb == nil && os.SameFile(sa, sb)) {
				return fmt.Errorf("input, library, and output paths must be different")
			}
		}
	}
	return nil
}
