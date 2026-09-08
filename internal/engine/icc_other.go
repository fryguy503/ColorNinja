//go:build !windows

package engine

import (
	"context"
	"fmt"
	"image"
)

func convertICC(ctx context.Context, src image.Image, profile []byte) (*image.NRGBA, error) {
	if converted, err, handled := convertMatrixICC(ctx, src, profile); handled {
		return converted, err
	}
	return nil, fmt.Errorf("ICC conversion currently requires Windows; export the input in sRGB first")
}
