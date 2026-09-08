package engine

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/disintegration/imaging"
	"golang.org/x/sys/windows"
	"image"
	"runtime"
	"unsafe"
)

var colorDLL = windows.NewLazySystemDLL("mscms.dll")
var openProfile = colorDLL.NewProc("OpenColorProfileW")
var closeProfile = colorDLL.NewProc("CloseColorProfile")
var createTransform = colorDLL.NewProc("CreateMultiProfileTransform")
var deleteTransform = colorDLL.NewProc("DeleteColorTransform")
var translateBits = colorDLL.NewProc("TranslateBitmapBits")

//go:embed srgb.icc
var standardSRGB []byte

type nativeProfile struct {
	kind uint32
	data unsafe.Pointer
	size uint32
}

// Windows' installed color-management engine handles arbitrary ICC LUT/matrix
// profiles. No Python, external DLL package, or C compiler is required.
func convertICC(ctx context.Context, src image.Image, profile []byte) (*image.NRGBA, error) {
	if converted, err, handled := convertMatrixICC(ctx, src, profile); handled {
		return converted, err
	}
	if len(profile) < 128 {
		return nil, fmt.Errorf("ICC profile is truncated")
	}
	native := nativeProfile{2, unsafe.Pointer(&profile[0]), uint32(len(profile))}
	source, _, e := openProfile.Call(uintptr(unsafe.Pointer(&native)), 1, 1, 3)
	runtime.KeepAlive(profile)
	if source == 0 {
		return nil, fmt.Errorf("open source ICC: %w", e)
	}
	defer closeProfile.Call(source)
	// Use a fixed sRGB profile; installed display profiles can vary by machine.
	targetBytes := standardSRGB
	destination := nativeProfile{2, unsafe.Pointer(&targetBytes[0]), uint32(len(targetBytes))}
	target, _, e := openProfile.Call(uintptr(unsafe.Pointer(&destination)), 1, 1, 3)
	runtime.KeepAlive(targetBytes)
	if target == 0 {
		return nil, fmt.Errorf("open sRGB profile: %w", e)
	}
	defer closeProfile.Call(target)
	handles := [2]uintptr{source, target}
	intent := uint32(0)
	const bestMode = 3
	transform, _, e := createTransform.Call(uintptr(unsafe.Pointer(&handles[0])), 2, uintptr(unsafe.Pointer(&intent)), 1, bestMode, 0)
	if transform == 0 {
		return nil, fmt.Errorf("create ICC transform: %w", e)
	}
	defer deleteTransform.Call(transform)
	normalized := imaging.Clone(src)
	w, h := normalized.Bounds().Dx(), normalized.Bounds().Dy()
	// BM_BGRTRIPLETS names packed channel significance. In little-endian
	// memory its bytes are R, G, B, matching our explicit byte buffers.
	inputFormat := uintptr(4)
	channels := 3
	var cmyk *image.CMYK
	if string(profile[16:20]) == "CMYK" {
		var ok bool
		cmyk, ok = src.(*image.CMYK)
		if !ok {
			return nil, fmt.Errorf("CMYK profile requires CMYK input pixels")
		}
		inputFormat = 0x0305 // BM_KYMCQUADS: C, M, Y, K bytes in memory
		channels = 4
	} else if string(profile[16:20]) == "GRAY" {
		inputFormat = 0x0209
		channels = 1
	} else if string(profile[16:20]) != "RGB " {
		return nil, fmt.Errorf("unsupported ICC input color model %q", profile[16:20])
	}
	rows := max(1, min(h, 131072/max(1, w)))
	inStride := (w*channels + 3) &^ 3
	outStride := (w*3 + 3) &^ 3
	input := make([]byte, inStride*rows)
	output := make([]byte, outStride*rows)
	for top := 0; top < h; top += rows {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		count := min(rows, h-top)
		for y := 0; y < count; y++ {
			for x := 0; x < w; x++ {
				i := (top+y)*normalized.Stride + x*4
				j := y*inStride + x*channels
				if cmyk != nil {
					p := cmyk.CMYKAt(x+cmyk.Rect.Min.X, top+y+cmyk.Rect.Min.Y)
					input[j], input[j+1], input[j+2], input[j+3] = p.C, p.M, p.Y, p.K
				} else if channels == 1 {
					input[j] = normalized.Pix[i]
				} else {
					copy(input[j:j+3], normalized.Pix[i:i+3])
				}
			}
		}
		ok, _, err := translateBits.Call(transform, uintptr(unsafe.Pointer(&input[0])), inputFormat, uintptr(w), uintptr(count), uintptr(inStride), uintptr(unsafe.Pointer(&output[0])), 4, uintptr(outStride), 0, 0)
		runtime.KeepAlive(input)
		runtime.KeepAlive(output)
		if ok == 0 {
			return nil, fmt.Errorf("translate ICC pixels: %w", err)
		}
		for y := 0; y < count; y++ {
			for x := 0; x < w; x++ {
				i := (top+y)*normalized.Stride + x*4
				j := y*outStride + x*3
				copy(normalized.Pix[i:i+3], output[j:j+3])
			}
		}
	}
	return normalized, nil
}
