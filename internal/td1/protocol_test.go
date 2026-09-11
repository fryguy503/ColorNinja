package td1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fragmentPort struct {
	input  []byte
	output bytes.Buffer
	chunk  int
	short  bool
}

func (p *fragmentPort) Read(b []byte) (int, error) {
	if len(p.input) == 0 {
		return 0, io.EOF
	}
	n := min(len(b), len(p.input), p.chunk)
	copy(b, p.input[:n])
	p.input = p.input[n:]
	return n, nil
}
func (p *fragmentPort) Write(b []byte) (int, error) {
	if p.short && len(b) > 2 {
		b = b[:2]
	}
	return p.output.Write(b)
}
func wireFixture(input string, chunk int) (*wire, *fragmentPort) {
	p := &fragmentPort{input: []byte(input), chunk: chunk, short: true}
	return &wire{port: p}, p
}

func TestMeasurementFormats(t *testing.T) {
	r, ok := ParseReading("11267973560,,,,6.9,530723")
	if !ok || r.TD != 6.9 || r.Color != "#530723" {
		t.Fatal("live TD1S fixture", r)
	}
	for _, line := range []string{"12,1,2,3,4.51,abcdef", "scan,,,,2.5,#102030", "12,1,2,3,4.51,abcdef,100,200,300"} {
		r, ok := ParseReading(line)
		if !ok || r.TD <= 0 || r.CapturedAt == "" {
			t.Fatalf("lost vendor format: %q %+v", line, r)
		}
	}
	for _, line := range []string{"display,1,2,3,4.5,abcdef", "1,2,3,4,NaN,ffffff", "1,2,3,4,+Inf,ffffff", "1,2,3,4,-1,ffffff", "1,2,3,4,0,ffffff", "1,2,3,4,2,gggggg", "1,2,3,4,2,abcdef,7", "version,TD1 Version: 2.0.2"} {
		if _, ok := ParseReading(line); ok {
			t.Fatalf("accepted %q", line)
		}
	}
}
func TestHandshakeSkipsAsyncDisplayAndMeasurements(t *testing.T) {
	w, p := wireFixture("display,Connecting,1,True\r\nclearScreen\nready\n1,2,3,4,5.2,abcdef\nconnected to PY licensed\n", 1)
	var readings int
	w.observe = func(line string) {
		if _, ok := ParseReading(line); ok {
			readings++
		}
	}
	r, e := w.data(context.Background(), "connect", "PY\n")
	if e != nil || r != "connected to PY licensed" || p.output.String() != "connect\nPY\n" || readings != 1 {
		t.Fatalf("%q %v writes=%q readings=%d", r, e, p.output.String(), readings)
	}
}
func TestReceiveBinaryBoundaries(t *testing.T) {
	for _, chunk := range []int{1, 2, 7, 4096} {
		t.Run(string(rune(chunk)), func(t *testing.T) {
			w, p := wireFixture("ready\n8\n2\n3\na\x00\n5\n\xffb\nc!after\n", chunk)
			raw, e := w.receive(context.Background(), "license.bin")
			if e != nil || !bytes.Equal(raw, []byte("a\x00\n\xffb\nc!")) {
				t.Fatalf("%x %v", raw, e)
			}
			line, e := w.line(context.Background())
			if e != nil || line != "after" {
				t.Fatalf("lost trailing line %q %v", line, e)
			}
			if p.output.String() != "retrieve file\nlicense.bin\nready\nready\n" {
				t.Fatal(p.output.String())
			}
		})
	}
}
func TestReceiveRejectsCorruptFraming(t *testing.T) {
	for _, input := range []string{"ready\n-1\n", "ready\n1048577\n", "ready\n2\n1\n3\nabc", "ready\n2\n1\n0\n", "ready\n8\n1\n3\nabc", "ready\n8\n1\n8\nab"} {
		w, _ := wireFixture(input, 4096)
		if _, e := w.receive(context.Background(), "settings.py"); e == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	w, _ := wireFixture("No file named errors.txt\n", 1)
	if _, e := w.receive(context.Background(), "errors.txt"); !errors.Is(e, ErrNoFile) {
		t.Fatal(e)
	}
}
func TestSendFileBlocksAndShortWrites(t *testing.T) {
	raw := bytes.Repeat([]byte{0, 10, 255}, 700)
	w, p := wireFixture("ready\nready\nready\nready\n", 4096)
	if e := w.sendFile(context.Background(), "lib/TD1.mpy", raw); e != nil {
		t.Fatal(e)
	}
	expected := append([]byte("file\nlib/TD1.mpy\n2100\n3\n1024\n"), raw[:1024]...)
	expected = append(expected, []byte("1024\n")...)
	expected = append(expected, raw[1024:2048]...)
	expected = append(expected, []byte("52\n")...)
	expected = append(expected, raw[2048:]...)
	if !bytes.Equal(expected, p.output.Bytes()) {
		t.Fatal("incorrect binary update framing")
	}
}
func TestCanceledReadsStopWithBufferedData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w, _ := wireFixture("", 1)
	w.pending = []byte("ready\n")
	if _, e := w.line(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestSettingsOnlyUsesKnownLiteralAssignments(t *testing.T) {
	raw := []byte("display_type = 'SH1106'\nRGB_Enabled = True # comment\nsample_rate = 10\ncontinuous_mode = True\ncontinuous_color = True\nunknown = do_thing()\n")
	values := ParseSettings(raw)
	command, e := SettingsCommand(values)
	if e != nil || !strings.Contains(command, `display_type = "SH1106"`) {
		t.Fatal(command, e)
	}
	for _, bad := range []map[string]string{{"RGB_Enabled": "True\ndone\nupdate"}, {"arbitrary": "1"}, {"sample_rate": "0"}, {"sample_rate": "61"}, {"display_type": "SH1106\"\nprint(1)"}, {"continuous_color": "True", "continuous_mode": "False"}} {
		if _, e := SettingsCommand(bad); e == nil {
			t.Fatal("accepted unsafe settings", bad)
		}
	}
}
func TestManagerSnapshotIsIsolatedAndBounded(t *testing.T) {
	m := New()
	for i := 0; i < 150; i++ {
		m.observe("1,2,3,4,5,abcdef")
		m.observe("status")
	}
	s := m.State()
	if len(s.Readings) != 100 || len(s.Messages) != 100 || s.Latest.ID != 150 {
		t.Fatal(s)
	}
	s.Latest.TD = 99
	s.Readings[0].TD = 99
	s.Messages[0] = "changed"
	if m.State().Latest.TD == 99 || m.State().Readings[0].TD == 99 || m.State().Messages[0] == "changed" {
		t.Fatal("snapshot aliases state")
	}
	if _, e := m.ReadFile("settings.py"); e == nil {
		t.Fatal("allowed read when disconnected")
	}
	m.Disconnect()
	time.Sleep(time.Millisecond)
}
func TestDisplayMirrorUsesCoordinatesAndClearScreen(t *testing.T) {
	m := New()
	m.observe("display,128, 64, 32, 0, 18")
	m.observe("display,TD 2.7,0,32")
	m.observe("display,TD 3.1,0,32")
	s := m.State()
	if len(s.Display) != 2 || s.Display[0].Text != "128, 64, 32" || s.Display[1].Text != "TD 3.1" {
		t.Fatal(s.Display)
	}
	m.observe("display,ClearScreen")
	if len(m.State().Display) != 0 {
		t.Fatal("did not clear mirror")
	}
	if _, _, ok := ParseDisplay("display,garbage,NaN,0"); ok {
		t.Fatal("invalid coordinate")
	}
}
func TestConnectedTD1SSettingsFixture(t *testing.T) {
	raw := []byte("scale_output = False\noptical_button = False\ncontinuous_start_time = 5\nscreen_mirror = False\ncontinuous_color = False\ndisplay_flip = True\nscale_adjustment = False\nsample_rate = 1\ndisplay_type = \"SSD1306\"\noutput_raw_rgb = False\nRGB_Enabled = True\nsample_distance = 1.75\ncontinuous_mode = False\ncolor_rgb = False\n")
	values := ParseSettings(raw)
	if len(values) != 14 {
		t.Fatal(values)
	}
	if _, e := SettingsCommand(values); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"NaN", "Inf", "0", "100", "1.75\nupdate"} {
		if _, e := SettingsCommand(map[string]string{"sample_distance": value}); e == nil {
			t.Fatal("unsafe sample distance", value)
		}
	}
}
