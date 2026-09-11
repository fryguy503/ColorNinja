//go:build darwin && !cgo

package td1

import "fmt"

// Native macOS desktop builds enable cgo for Wails and USB IOKit enumeration.
// Keep pure-Go CLI/cross builds usable without importing the cgo enumerator.
func Ports() ([]Port, error) {
	return nil, fmt.Errorf("TD1 USB discovery on macOS requires a native build with CGO_ENABLED=1")
}
