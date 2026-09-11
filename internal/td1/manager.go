package td1

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

var ErrNoFile = errors.New("file not present on device")

type Port struct {
	Name    string `json:"name"`
	Serial  string `json:"serial"`
	Product string `json:"product"`
}

type State struct {
	Connected bool          `json:"connected"`
	Port      Port          `json:"port"`
	Status    string        `json:"status"`
	Error     string        `json:"error"`
	Identity  string        `json:"identity"`
	Version   string        `json:"version"`
	Latest    *Reading      `json:"latest"`
	Readings  []Reading     `json:"readings"`
	Messages  []string      `json:"messages"`
	Busy      bool          `json:"busy"`
	Display   []DisplayLine `json:"display"`
}
type DisplayLine struct {
	Text string `json:"text"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}
type operation struct {
	fn      func(context.Context, *wire) (any, error)
	result  chan outcome
	timeout time.Duration
}
type outcome struct {
	value any
	err   error
}
type Manager struct {
	mu        sync.Mutex
	lifecycle sync.Mutex
	state     State
	cancel    context.CancelFunc
	done      chan struct{}
	commands  chan operation
	sequence  uint64
}

func New() *Manager {
	return &Manager{state: State{Status: "Connect a TD1 or TD1S to measure filament", Readings: []Reading{}, Messages: []string{}}}
}
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.Messages = append([]string{}, s.Messages...)
	s.Readings = append([]Reading{}, s.Readings...)
	s.Display = append([]DisplayLine{}, s.Display...)
	if s.Latest != nil {
		r := *s.Latest
		s.Latest = &r
	}
	return s
}
func (m *Manager) observe(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if display, clear, ok := ParseDisplay(line); ok {
		if clear {
			m.state.Display = []DisplayLine{}
			return
		}
		for i, d := range m.state.Display {
			if d.X == display.X && d.Y == display.Y {
				m.state.Display[i] = display
				return
			}
		}
		if len(m.state.Display) < 64 {
			m.state.Display = append(m.state.Display, display)
		}
		return
	}
	if reading, ok := ParseReading(line); ok {
		m.sequence++
		reading.ID = m.sequence
		reading.Serial = m.state.Port.Serial
		m.state.Latest = &reading
		m.state.Readings = append(m.state.Readings, reading)
		if len(m.state.Readings) > 100 {
			m.state.Readings = m.state.Readings[1:]
		}
		m.state.Status = "Measurement received"
		return
	}
	if line != "" {
		// Keep unsolicited diagnostics bounded independently of file transfers.
		if len(line) > 4096 {
			line = line[:4096] + "…"
		}
		m.state.Messages = append(m.state.Messages, line)
		if len(m.state.Messages) > 100 {
			m.state.Messages = m.state.Messages[1:]
		}
		if line != "ready" && !m.state.Busy {
			m.state.Status = line
		}
	}
}
func (m *Manager) Disconnect() { m.lifecycle.Lock(); defer m.lifecycle.Unlock(); m.disconnect() }
func (m *Manager) disconnect() {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	m.mu.Lock()
	m.cancel = nil
	m.state.Connected = false
	m.state.Busy = false
	m.state.Status = "Disconnected"
	m.mu.Unlock()
}
func (m *Manager) Connect(name string) (State, error) {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.disconnect()
	ps, err := Ports()
	if err != nil {
		return m.State(), err
	}
	var selected *Port
	for _, p := range ps {
		if p.Name == name {
			cp := p
			selected = &cp
		}
	}
	if selected == nil {
		return m.State(), fmt.Errorf("TD1/TD1S is not present on %s; refresh USB devices", name)
	}
	p, err := serial.Open(name, &serial.Mode{BaudRate: 115200, DataBits: 8, StopBits: serial.OneStopBit, Parity: serial.NoParity, InitialStatusBits: &serial.ModemOutputBits{DTR: true, RTS: true}})
	if err != nil {
		return m.State(), fmt.Errorf("open TD1: %w; close HueForge or other TD1 applications", err)
	}
	if err = p.SetReadTimeout(100 * time.Millisecond); err != nil {
		p.Close()
		return m.State(), err
	}
	_ = p.ResetInputBuffer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	commands := make(chan operation)
	m.mu.Lock()
	m.state = State{Port: *selected, Status: "Connecting", Busy: true, Readings: []Reading{}, Messages: []string{}}
	m.cancel = cancel
	m.done = done
	m.commands = commands
	m.mu.Unlock()
	connected := make(chan error, 1)
	go func() {
		defer close(done)
		defer p.Close()
		w := &wire{port: p, observe: m.observe}
		handshake, hcancel := context.WithTimeout(ctx, 12*time.Second)
		stopHandshakeClose := context.AfterFunc(handshake, func() { _ = p.Close() })
		identity, e := w.data(handshake, "connect", "PY\n")
		stopHandshakeClose()
		hcancel()
		if e == nil && !strings.HasPrefix(strings.ToLower(identity), "connected to py") {
			e = fmt.Errorf("unexpected TD1 handshake: %s", identity)
		}
		m.mu.Lock()
		m.state.Identity = identity
		m.state.Connected = e == nil
		m.state.Busy = false
		m.state.Status = "Connected"
		if e != nil {
			m.state.Error = e.Error()
			m.state.Status = "Connection failed"
		}
		m.mu.Unlock()
		connected <- e
		if e != nil {
			return
		}
		defer func() { m.mu.Lock(); m.state.Connected = false; m.state.Busy = false; m.mu.Unlock() }()
		for ctx.Err() == nil {
			select {
			case op := <-commands:
				m.mu.Lock()
				m.state.Busy = true
				m.state.Error = ""
				m.state.Status = "Communicating with TD1/S"
				m.mu.Unlock()
				opctx, stop := context.WithTimeout(ctx, op.timeout)
				stopOperationClose := context.AfterFunc(opctx, func() { _ = p.Close() })
				v, e := op.fn(opctx, w)
				stopOperationClose()
				stop()
				m.mu.Lock()
				m.state.Busy = false
				if m.state.Status == "Communicating with TD1/S" || strings.HasPrefix(m.state.Status, "Sending ") {
					m.state.Status = "Ready"
				}
				if e != nil && !errors.Is(e, ErrNoFile) {
					m.state.Error = e.Error()
				}
				m.mu.Unlock()
				op.result <- outcome{v, e}
				// A timed-out transfer has unknown framing. Reconnect before any
				// further commands, rather than treating binary bytes as readings.
				if e != nil && !errors.Is(e, ErrNoFile) {
					return
				}
			default:
				poll, stop := context.WithTimeout(ctx, 150*time.Millisecond)
				_, e := w.line(poll)
				stop()
				if e != nil && !errors.Is(e, context.DeadlineExceeded) && !errors.Is(e, context.Canceled) {
					m.mu.Lock()
					m.state.Error = "TD1 disconnected: " + e.Error()
					m.state.Latest = nil
					m.mu.Unlock()
					return
				}
			}
		}
	}()
	err = <-connected
	return m.State(), err
}
func (m *Manager) run(timeout time.Duration, fn func(context.Context, *wire) (any, error)) (any, error) {
	m.mu.Lock()
	connected, commands, done := m.state.Connected, m.commands, m.done
	m.mu.Unlock()
	if !connected {
		return nil, fmt.Errorf("connect a TD1 or TD1S first")
	}
	op := operation{fn: fn, result: make(chan outcome, 1), timeout: timeout}
	select {
	case commands <- op:
	case <-done:
		return nil, fmt.Errorf("TD1 disconnected")
	}
	select {
	case out := <-op.result:
		return out.value, out.err
	case <-done:
		select {
		case out := <-op.result:
			return out.value, out.err
		default:
			return nil, fmt.Errorf("TD1 disconnected")
		}
	}
}
func (m *Manager) ReadFile(name string) ([]byte, error) {
	allowed := map[string]bool{"settings.py": true, "errors.txt": true, "boot_out.txt": true, "version.json": true, "license.bin": true, "pins.py": true, "lib/rgbOffset.py": true, "emptyLux.txt": true}
	if !allowed[name] {
		return nil, fmt.Errorf("unsupported TD1 diagnostic file")
	}
	// Device find_file searches recursively by basename.
	if name == "lib/rgbOffset.py" {
		name = "rgbOffset.py"
	}
	v, err := m.run(15*time.Second, func(ctx context.Context, w *wire) (any, error) { return w.receive(ctx, name) })
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}
func (m *Manager) Action(action string) (string, error) {
	if action == "reboot" {
		raw, err := m.ReadFile("settings.py")
		if err != nil {
			return "", err
		}
		value, ok := ParseSettings(raw)["RGB_Enabled"]
		if !ok {
			return "", fmt.Errorf("device does not advertise RGB_Enabled; reconnect its USB cable to reboot")
		}
		return m.ApplySettings(map[string]string{"RGB_Enabled": value})
	}
	commands := map[string]string{"version": "version", "calibrate-lux": "calibrate emptyLux", "calibrate-rgb": "rgb cal\nrgb", "reset-rgb": "rgb cal\nmatrix", "glow": "glow", "bootloader": "bootloader"}
	command, ok := commands[action]
	if !ok {
		return "", fmt.Errorf("unsupported TD1 action")
	}
	if action == "bootloader" {
		version, err := m.Action("version")
		if err != nil {
			return "", err
		}
		if !versionAtLeast(Versions(version)["comms.mpy"], "1.0.4") {
			return "", fmt.Errorf("this firmware does not advertise bootloader-command support; use the vendor's physical recovery procedure")
		}
	}
	timeout := 15 * time.Second
	if action == "calibrate-rgb" || action == "reset-rgb" {
		timeout = 5 * time.Minute
	}
	v, err := m.run(timeout, func(ctx context.Context, w *wire) (any, error) {
		if action == "bootloader" {
			if err := w.send([]byte(command + "\n")); err != nil {
				return nil, err
			}
			return "Bootloader requested. Disconnect here, then follow the vendor recovery instructions.", nil
		}
		return w.command(ctx, command)
	})
	if err != nil {
		return "", err
	}
	reply := v.(string)
	if action == "version" {
		m.mu.Lock()
		m.state.Version = reply
		m.mu.Unlock()
	}
	return reply, nil
}
func (m *Manager) ApplySettings(values map[string]string) (string, error) {
	data, err := SettingsCommand(values)
	if err != nil {
		return "", err
	}
	v, err := m.run(15*time.Second, func(ctx context.Context, w *wire) (any, error) { return w.data(ctx, "change settings", data) })
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
func (m *Manager) AdjustRGB(red, green, blue int) (string, error) {
	for _, n := range []int{red, green, blue} {
		if n < -100 || n > 100 {
			return "", fmt.Errorf("RGB adjustment must be -100 to 100 percent")
		}
	}
	v, err := m.run(15*time.Second, func(ctx context.Context, w *wire) (any, error) {
		return w.data(ctx, "rgb adj", fmt.Sprintf("%d\n%d\n%d\n", red, green, blue))
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
