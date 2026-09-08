package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/disintegration/imaging"
	"github.com/mandykoh/prism/meta/autometa"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type ImageMetadata struct {
	Format       string     `json:"format"`
	DPI          [2]float64 `json:"dpi"`
	ColorProfile string     `json:"colorProfile"`
	Warnings     []string   `json:"warnings"`
}
type LoadedImage struct {
	Image    *image.NRGBA
	Metadata ImageMetadata
}

const MaxImagePixels = 100_000_000

func LoadImage(ctx context.Context, path string) (*LoadedImage, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("open image: %w", e)
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if info.Size() > 512*1024*1024 {
		return nil, fmt.Errorf("image file exceeds the 512 MB input limit")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 512*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 512*1024*1024 {
		return nil, fmt.Errorf("image file exceeds the 512 MB input limit")
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	decodeData, offset, canvas, e := firstWebPFrame(raw)
	if e != nil {
		return nil, e
	}
	config, format, e := image.DecodeConfig(bytes.NewReader(decodeData))
	if e != nil {
		return nil, fmt.Errorf("unsupported or damaged image: %w", e)
	}
	if config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > MaxImagePixels || int64(canvas.X)*int64(canvas.Y) > MaxImagePixels {
		return nil, fmt.Errorf("image exceeds the 100 megapixel limit")
	}
	decoded, _, e := image.Decode(bytes.NewReader(decodeData))
	if e != nil {
		return nil, fmt.Errorf("decode image: %w", e)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if format == "webp" {
		decoded, e = normalizeWebP(ctx, decoded)
		if e != nil {
			return nil, e
		}
	}
	metadata := ImageMetadata{Format: format, ColorProfile: "sRGB", Warnings: []string{}}
	orientation, dpi, tiffICC := embeddedMetadata(raw)
	metadata.DPI = dpi
	profile := tiffICC
	if md, _, me := autometa.Load(bytes.NewReader(raw)); me == nil {
		p, pe := md.ICCProfileData()
		if pe != nil {
			metadata.Warnings = append(metadata.Warnings, "Embedded color profile could not be read; assuming sRGB.")
		} else if len(p) > 0 {
			profile = p
		}
	}
	normalized := imaging.Clone(decoded)
	if len(profile) > 0 {
		converted, ce := convertICC(ctx, decoded, profile)
		if ce != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			metadata.Warnings = append(metadata.Warnings, "Embedded ICC conversion failed; assuming sRGB: "+ce.Error())
			metadata.ColorProfile = "sRGB (assumed)"
		} else {
			normalized = converted
			metadata.ColorProfile = "Converted to sRGB"
		}
	}
	animatedWebP := canvas.X > 0
	if format == "gif" {
		canvas = image.Pt(config.Width, config.Height)
		offset = decoded.Bounds().Min
	}
	if canvas.X > 0 {
		if offset.X+normalized.Bounds().Dx() > canvas.X || offset.Y+normalized.Bounds().Dy() > canvas.Y {
			return nil, fmt.Errorf("image frame extends beyond its canvas")
		}
		full := image.NewNRGBA(image.Rect(0, 0, canvas.X, canvas.Y))
		for y := 0; y < normalized.Bounds().Dy(); y++ {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			dy := y + offset.Y
			if dy < 0 || dy >= canvas.Y {
				continue
			}
			for x := 0; x < normalized.Bounds().Dx(); x++ {
				dx := x + offset.X
				if dx < 0 || dx >= canvas.X {
					continue
				}
				s := y*normalized.Stride + x*4
				d := dy*full.Stride + dx*4
				copy(full.Pix[d:d+4], normalized.Pix[s:s+4])
			}
		}
		normalized = full
		if animatedWebP {
			metadata.Warnings = append(metadata.Warnings, "Animated WebP: using the first frame.")
		}
	}
	switch orientation {
	case 2:
		normalized = imaging.FlipH(normalized)
	case 3:
		normalized = imaging.Rotate180(normalized)
	case 4:
		normalized = imaging.FlipV(normalized)
	case 5:
		normalized = imaging.Transpose(normalized)
	case 6:
		normalized = imaging.Rotate270(normalized)
	case 7:
		normalized = imaging.Transverse(normalized)
	case 8:
		normalized = imaging.Rotate90(normalized)
	}
	if orientation >= 5 && orientation <= 8 {
		metadata.DPI[0], metadata.DPI[1] = metadata.DPI[1], metadata.DPI[0]
	}
	if format == "gif" {
		metadata.Warnings = append(metadata.Warnings, "GIF: using the first frame.")
	}
	return &LoadedImage{normalized, metadata}, ctx.Err()
}

// VP8 uses limited-range BT.601. The Go WebP decoder returns those original
// YUV planes in image.YCbCr, whose default RGB conversion assumes JPEG's full
// range. Convert explicitly so WebP imports retain their contrast and hue.
func normalizeWebP(ctx context.Context, decoded image.Image) (image.Image, error) {
	var planes *image.YCbCr
	var alpha *image.NYCbCrA
	switch img := decoded.(type) {
	case *image.YCbCr:
		planes = img
	case *image.NYCbCrA:
		planes, alpha = &img.YCbCr, img
	default:
		return decoded, nil // Lossless VP8L is already RGB.
	}
	b := planes.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	clip := func(v int) byte { return byte(max(0, min(255, v))) }
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			ci := planes.COffset(x, y)
			l, u, v := int(planes.Y[planes.YOffset(x, y)])-16, int(planes.Cb[ci])-128, int(planes.Cr[ci])-128
			i := (y-b.Min.Y)*out.Stride + (x-b.Min.X)*4
			out.Pix[i] = clip((298*l + 409*v + 128) >> 8)
			out.Pix[i+1] = clip((298*l - 100*u - 208*v + 128) >> 8)
			out.Pix[i+2] = clip((298*l + 516*u + 128) >> 8)
			out.Pix[i+3] = 255
			if alpha != nil {
				out.Pix[i+3] = alpha.A[alpha.AOffset(x, y)]
			}
		}
	}
	return out, nil
}

func embeddedMetadata(raw []byte) (int, [2]float64, []byte) {
	orientation := 1
	dpi := [2]float64{}
	var icc []byte
	parse := func(data []byte) {
		o, d, p := parseTIFF(bytes.TrimPrefix(data, []byte("Exif\x00\x00")))
		if o != 0 {
			orientation = o
		}
		if d[0] > 0 && d[1] > 0 {
			dpi = d
		}
		if len(p) > 0 {
			icc = p
		}
	}
	if len(raw) >= 8 && (string(raw[:2]) == "II" || string(raw[:2]) == "MM") {
		parse(raw)
	}
	if len(raw) > 8 && string(raw[:8]) == "\x89PNG\r\n\x1a\n" {
		for pos := 8; pos+12 <= len(raw); {
			n := int(binary.BigEndian.Uint32(raw[pos:]))
			if n > len(raw)-pos-12 {
				break
			}
			kind := string(raw[pos+4 : pos+8])
			data := raw[pos+8 : pos+8+n]
			if kind == "eXIf" {
				parse(data)
			}
			if kind == "pHYs" && n == 9 && data[8] == 1 {
				dpi = [2]float64{float64(binary.BigEndian.Uint32(data)) * .0254, float64(binary.BigEndian.Uint32(data[4:])) * .0254}
			}
			pos += 12 + n
		}
	}
	if len(raw) > 2 && raw[0] == 255 && raw[1] == 216 {
		for pos := 2; pos+4 < len(raw); {
			if raw[pos] != 255 {
				break
			}
			marker := raw[pos+1]
			if marker == 218 || marker == 217 {
				break
			}
			n := int(binary.BigEndian.Uint16(raw[pos+2:]))
			if n < 2 || pos+2+n > len(raw) {
				break
			}
			d := raw[pos+4 : pos+2+n]
			if marker == 225 && bytes.HasPrefix(d, []byte("Exif\x00\x00")) {
				parse(d)
			}
			if marker == 224 && len(d) >= 12 && string(d[:5]) == "JFIF\x00" {
				factor := 0.
				if d[7] == 1 {
					factor = 1
				} else if d[7] == 2 {
					factor = 2.54
				}
				if factor > 0 {
					dpi = [2]float64{float64(binary.BigEndian.Uint16(d[8:])) * factor, float64(binary.BigEndian.Uint16(d[10:])) * factor}
				}
			}
			pos += 2 + n
		}
	}
	if len(raw) > 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		for pos := 12; pos+8 <= len(raw); {
			n := int(binary.LittleEndian.Uint32(raw[pos+4:]))
			if n > len(raw)-pos-8 {
				break
			}
			if string(raw[pos:pos+4]) == "EXIF" {
				parse(raw[pos+8 : pos+8+n])
			}
			pos += 8 + n + n%2
		}
	}
	return orientation, dpi, icc
}
func parseTIFF(data []byte) (int, [2]float64, []byte) {
	var order binary.ByteOrder
	if len(data) < 8 {
		return 0, [2]float64{}, nil
	}
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, [2]float64{}, nil
	}
	if order.Uint16(data[2:]) != 42 {
		return 0, [2]float64{}, nil
	}
	pos := int(order.Uint32(data[4:]))
	if pos > len(data)-2 {
		return 0, [2]float64{}, nil
	}
	count := int(order.Uint16(data[pos:]))
	orientation := 0
	dpi := [2]float64{}
	unit := 2
	var icc []byte
	for i := 0; i < count; i++ {
		p := pos + 2 + i*12
		if p > len(data)-12 {
			break
		}
		tag, typ, n := order.Uint16(data[p:]), order.Uint16(data[p+2:]), int(order.Uint32(data[p+4:]))
		size := map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1}[typ]
		if size == 0 || n > len(data)/size {
			continue
		}
		length := n * size
		start := p + 8
		if length > 4 {
			start = int(order.Uint32(data[p+8:]))
		}
		if start > len(data)-length {
			continue
		}
		v := data[start : start+length]
		if tag == 274 && typ == 3 && len(v) >= 2 {
			orientation = int(order.Uint16(v))
		}
		if tag == 296 && typ == 3 && len(v) >= 2 {
			unit = int(order.Uint16(v))
		}
		if (tag == 282 || tag == 283) && typ == 5 && len(v) >= 8 {
			num, den := order.Uint32(v), order.Uint32(v[4:])
			if den > 0 {
				dpi[int(tag-282)] = float64(num) / float64(den)
			}
		}
		if tag == 34675 {
			icc = append([]byte{}, v...)
		}
	}
	if unit == 3 {
		dpi[0] *= 2.54
		dpi[1] *= 2.54
	} else if unit != 2 {
		dpi = [2]float64{}
	}
	return orientation, dpi, icc
}
func firstWebPFrame(raw []byte) ([]byte, image.Point, image.Point, error) {
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		return raw, image.Point{}, image.Point{}, nil
	}
	canvas := image.Point{}
	u24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
	for pos := 12; pos+8 <= len(raw); {
		n := int(binary.LittleEndian.Uint32(raw[pos+4:]))
		if n > len(raw)-pos-8 {
			return nil, image.Point{}, canvas, fmt.Errorf("truncated WebP chunk")
		}
		kind := string(raw[pos : pos+4])
		d := raw[pos+8 : pos+8+n]
		if kind == "VP8X" && n >= 10 {
			canvas = image.Pt(u24(d[4:])+1, u24(d[7:])+1)
		}
		if kind == "ANMF" && n >= 16 {
			offset := image.Pt(2*u24(d), 2*u24(d[3:]))
			if canvas.X == 0 || offset.X+u24(d[6:])+1 > canvas.X || offset.Y+u24(d[9:])+1 > canvas.Y {
				return nil, offset, canvas, fmt.Errorf("WebP frame extends beyond its canvas")
			}
			frame := d[16:]
			out := make([]byte, 12, len(frame)+30)
			copy(out, "RIFF\x00\x00\x00\x00WEBP")
			if bytes.HasPrefix(frame, []byte("ALPH")) {
				header := make([]byte, 18)
				copy(header, "VP8X")
				binary.LittleEndian.PutUint32(header[4:], 10)
				header[8] = 16
				copy(header[12:15], d[6:9])
				copy(header[15:18], d[9:12])
				out = append(out, header...)
			}
			out = append(out, frame...)
			binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
			return out, offset, canvas, nil
		}
		pos += 8 + n + n%2
	}
	return raw, image.Point{}, image.Point{}, nil
}
