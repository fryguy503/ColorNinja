package main

import (
	"colorninja/internal/studio"
	"colorninja/internal/td1"
	"fmt"
)

func integrationAPI(s *studio.Studio, method string, get func(int, any) error) (any, error) {
	switch method {
	case "ImportCopiedFilamentProfile":
		var value, content string
		if e := get(0, &value); e != nil {
			return nil, e
		}
		if e := get(1, &content); e != nil {
			return nil, e
		}
		return s.ImportCopiedFilamentProfile(value, content)
	case "TD1Ports":
		return td1.Ports()
	case "TD1State":
		return s.TD1.State(), nil
	case "DisconnectTD1":
		s.DisconnectTD1()
		return nil, nil
	case "ConnectTD1":
		var p string
		if e := get(0, &p); e != nil {
			return nil, e
		}
		return s.ConnectTD1(p)
	case "TD1Operation":
		var r studio.TD1Request
		if e := get(0, &r); e != nil {
			return nil, e
		}
		return s.TD1Operation(r)
	case "FilamentCatalog":
		var p string
		if e := get(0, &p); e != nil {
			return nil, e
		}
		return s.FilamentCatalog(p)
	case "EditFilament":
		var r studio.FilamentEdit
		if e := get(0, &r); e != nil {
			return nil, e
		}
		return s.EditFilament(r)
	case "ImportFilamentProfiles":
		var content, format string
		if e := get(0, &content); e != nil {
			return nil, e
		}
		if e := get(1, &format); e != nil {
			return nil, e
		}
		return s.ImportFilamentProfiles(content, format)
	case "ExportFilamentLibrary":
		var p, d string
		if e := get(0, &p); e != nil {
			return nil, e
		}
		if e := get(1, &d); e != nil {
			return nil, e
		}
		return d, s.ExportFilamentLibrary(p, d, false)
	}
	return nil, fmt.Errorf("unknown integration method")
}
