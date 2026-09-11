package td1

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var booleanSettings = map[string]bool{"display_flip": true, "color_rgb": true, "continuous_mode": true, "continuous_color": true, "RGB_Enabled": true, "optical_button": true, "output_raw_rgb": true, "scale_adjustment": true, "scale_output": true, "screen_mirror": true}

func ParseSettings(raw []byte) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.SplitN(line, "#", 2)[0]
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 {
			continue
		}
		key, value := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
		if booleanSettings[key] || key == "sample_rate" || key == "continuous_start_time" || key == "sample_distance" || key == "display_type" {
			values[key] = value
		}
	}
	return values
}
func SettingsCommand(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("no settings changes")
	}
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		value := values[key]
		if booleanSettings[key] {
			if value != "True" && value != "False" {
				return "", fmt.Errorf("invalid %s", key)
			}
		} else if key == "display_type" {
			value = strings.Trim(value, "\"'")
			if value != "SH1106" && value != "SSD1306" {
				return "", fmt.Errorf("unsupported display type")
			}
			value = strconv.Quote(value)
		} else if key == "sample_distance" {
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || !(n >= 0.1 && n <= 10) {
				return "", fmt.Errorf("sample distance must be 0.1–10 mm")
			}
			value = strconv.FormatFloat(n, 'f', -1, 64)
		} else {
			n, err := strconv.Atoi(value)
			if err != nil {
				return "", fmt.Errorf("invalid %s", key)
			}
			switch key {
			case "sample_rate", "continuous_start_time":
				if n < 1 || n > 60 {
					return "", fmt.Errorf("scan interval and start delay must be 1–60 seconds")
				}
			default:
				return "", fmt.Errorf("unsupported TD1 setting %s", key)
			}
		}
		fmt.Fprintf(&b, "%s = %s\n", key, value)
	}
	if values["continuous_color"] == "True" && values["continuous_mode"] == "False" {
		return "", fmt.Errorf("continuous color requires continuous TD")
	}
	b.WriteString("done\n")
	return b.String(), nil
}
