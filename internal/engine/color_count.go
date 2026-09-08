package engine

import (
	"context"
	"fmt"
	"image"
)

// CountUniqueColors counts distinct 8-bit RGB triples with nonzero alpha.
// Different alpha values do not multiply the color count. A 2 MiB bitset
// bounds memory regardless of image dimensions or the number of colors.
func CountUniqueColors(ctx context.Context, img *image.NRGBA) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if img == nil {
		return 0, fmt.Errorf("no image loaded")
	}
	seen := make([]uint64, 1<<18)
	count := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < img.Bounds().Dx(); x++ {
			if x%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
			}
			i := x * 4
			if row[i+3] == 0 {
				continue
			}
			rgb := uint32(row[i])<<16 | uint32(row[i+1])<<8 | uint32(row[i+2])
			word, bit := rgb>>6, uint64(1)<<(rgb&63)
			if seen[word]&bit == 0 {
				seen[word] |= bit
				count++
			}
		}
	}
	return count, ctx.Err()
}

// Mapping already records each palette entry's visible pixel coverage, so
// the result needs no second image scan. Unused entries do not count.
func usedPaletteColorCount(palette []PaletteEntry) int {
	seen := make(map[RGB]struct{}, len(palette))
	for _, color := range palette {
		if color.PixelFraction > 0 {
			seen[color.RGB] = struct{}{}
		}
	}
	return len(seen)
}
