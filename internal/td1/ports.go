//go:build !darwin || cgo

package td1

import (
	"go.bug.st/serial/enumerator"
	"strings"
)

func Ports() ([]Port, error) {
	ps, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	result := []Port{}
	for _, p := range ps {
		if p.IsUSB && strings.EqualFold(p.VID, "E4B2") && strings.EqualFold(p.PID, "0045") {
			result = append(result, Port{p.Name, p.SerialNumber, p.Product})
		}
	}
	return result, nil
}
