package main

import (
	"context"
	"errors"
	"sync"

	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

// Backend abstracts all mouse operations so the UI can run against either real Linux hidraw
// hardware or an in-memory mock in ui.NewTester unit tests.
type Backend interface {
	Connect() (*hid.MouseState, error)
	Refresh() (*hid.MouseState, error)
	ReadBattery() (int, error)
	Close() error

	SetDPI(profile uint8, cfg hid.DPIConfig) error
	SetReportRate(hz int) error
	SetSilentHeight(level uint8) error
	SetLineRepair(on bool) error
	SetWaveRepair(on bool) error
	SetMotionSync(on bool) error
	SetFPS20000(on bool) error
	SetLowLatency(on bool) error
	SetSensorAngle(deg int8) error
	SetDebounce(ms uint8) error
	SetScrollUpTime(ms uint8) error
	SetSleep24G(sec uint16) error
	SetSleepBT(sec uint16) error
	SetCurrentProfile(profile uint8) (*hid.MouseState, error)
	SetKeySimple(btnIdx uint8, action [4]byte) error
	ResetDefaultKeys() ([16][4]byte, error)
	FactoryReset() (*hid.MouseState, error)
	CheckFirmware(ctx context.Context, devID uint32, currentFW uint16) hid.FirmwareCheckResult

	SetTrue8K(on bool) error
	ApplyPerformanceMode(mode hid.PerformanceMode) error
	SetAllKeys(profile uint8, matrix [16][4]byte) error
	ReadMacro(slot uint8) (hid.Macro, error)
	WriteMacro(slot uint8, m hid.Macro) error
	ApplyProfileSnapshot(prof SavedProfile) (*hid.MouseState, error)
}

// LinuxHIDBackend connects to real Valkyrie mice via /dev/hidraw*.
type LinuxHIDBackend struct {
	mu     sync.Mutex
	client *hid.Client
}

func NewLinuxHIDBackend() *LinuxHIDBackend {
	return &LinuxHIDBackend{}
}

func (b *LinuxHIDBackend) Connect() (*hid.MouseState, error) {
	b.mu.Lock()
	if b.client != nil {
		_ = b.client.Close()
		b.client = nil
	}
	b.mu.Unlock()

	devices, err := hid.Discover()
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, errors.New("未检测到 Valkyrie 鼠标（请确认已开机并通过蓝牙 / 2.4G / USB 连接）")
	}

	var lastErr error
	for _, dev := range devices {
		tr, err := hid.OpenTransport(dev)
		if err != nil {
			lastErr = err
			continue
		}
		client := hid.NewClient(tr)
		state, err := client.ReadAll()
		if err != nil {
			_ = client.Close()
			lastErr = err
			continue
		}
		b.mu.Lock()
		b.client = client
		b.mu.Unlock()
		return state, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("未能与检测到的设备建立通信")
}

func (b *LinuxHIDBackend) getClient() (*hid.Client, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client == nil {
		return nil, errors.New("鼠标未连接")
	}
	return b.client, nil
}

func (b *LinuxHIDBackend) Refresh() (*hid.MouseState, error) {
	c, err := b.getClient()
	if err != nil {
		return b.Connect()
	}
	st, err := c.ReadAll()
	if err != nil {
		// Try re-discovering once in case hidraw node changed after sleep/reconnect
		return b.Connect()
	}
	return st, nil
}

func (b *LinuxHIDBackend) ReadBattery() (int, error) {
	c, err := b.getClient()
	if err != nil {
		return -1, err
	}
	return c.ReadBattery()
}

func (b *LinuxHIDBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client != nil {
		err := b.client.Close()
		b.client = nil
		return err
	}
	return nil
}

func (b *LinuxHIDBackend) SetDPI(profile uint8, cfg hid.DPIConfig) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetDPI(profile, cfg)
}

func (b *LinuxHIDBackend) SetReportRate(hz int) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetReportRate(hz)
}

func (b *LinuxHIDBackend) SetSilentHeight(level uint8) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetSilentHeight(level)
}

func (b *LinuxHIDBackend) SetLineRepair(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetLineRepair(on)
}

func (b *LinuxHIDBackend) SetWaveRepair(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetWaveRepair(on)
}

func (b *LinuxHIDBackend) SetMotionSync(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetMotionSync(on)
}

func (b *LinuxHIDBackend) SetFPS20000(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetFPS20000(on)
}

func (b *LinuxHIDBackend) SetLowLatency(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetLowLatency(on)
}

func (b *LinuxHIDBackend) SetSensorAngle(deg int8) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetSensorAngle(deg)
}

func (b *LinuxHIDBackend) SetDebounce(ms uint8) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetDebounce(ms)
}

func (b *LinuxHIDBackend) SetScrollUpTime(ms uint8) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetScrollUpTime(ms)
}

func (b *LinuxHIDBackend) SetSleep24G(sec uint16) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetSleep24G(sec)
}

func (b *LinuxHIDBackend) SetSleepBT(sec uint16) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetSleepBT(sec)
}

func (b *LinuxHIDBackend) SetCurrentProfile(profile uint8) (*hid.MouseState, error) {
	c, err := b.getClient()
	if err != nil {
		return nil, err
	}
	if err := c.SetCurrentProfile(profile); err != nil {
		return nil, err
	}
	return c.ReadAll()
}

func (b *LinuxHIDBackend) SetKeySimple(btnIdx uint8, action [4]byte) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetKeySimple(btnIdx, action)
}

func (b *LinuxHIDBackend) ResetDefaultKeys() ([16][4]byte, error) {
	c, err := b.getClient()
	if err != nil {
		return [16][4]byte{}, err
	}
	for _, slot := range hid.ConfigurableSlots {
		if err := c.SetKeySimple(slot.Index, slot.DefaultAction); err != nil {
			return [16][4]byte{}, err
		}
	}
	params, err := c.ReadAllParams()
	if err != nil {
		return [16][4]byte{}, err
	}
	return c.ReadKeyMatrix(params.Profile)
}

func (b *LinuxHIDBackend) FactoryReset() (*hid.MouseState, error) {
	c, err := b.getClient()
	if err != nil {
		return nil, err
	}
	if err := c.FactoryReset(); err != nil {
		return nil, err
	}
	return c.ReadAll()
}

func (b *LinuxHIDBackend) CheckFirmware(ctx context.Context, devID uint32, currentFW uint16) hid.FirmwareCheckResult {
	return hid.CheckLatestFirmware(ctx, devID, currentFW)
}

func (b *LinuxHIDBackend) SetTrue8K(on bool) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetTrue8K(on)
}

func (b *LinuxHIDBackend) ApplyPerformanceMode(mode hid.PerformanceMode) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.ApplyPerformanceMode(mode)
}

func (b *LinuxHIDBackend) SetAllKeys(profile uint8, matrix [16][4]byte) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.SetAllKeys(profile, matrix)
}

func (b *LinuxHIDBackend) ReadMacro(slot uint8) (hid.Macro, error) {
	c, err := b.getClient()
	if err != nil {
		return hid.Macro{}, err
	}
	return c.ReadMacro(slot)
}

func (b *LinuxHIDBackend) WriteMacro(slot uint8, m hid.Macro) error {
	c, err := b.getClient()
	if err != nil {
		return err
	}
	return c.WriteMacro(slot, m)
}

// ApplyProfileSnapshot writes a saved profile back to the mouse and returns a fresh read.
// It does not factory-reset the device.
func (b *LinuxHIDBackend) ApplyProfileSnapshot(prof SavedProfile) (*hid.MouseState, error) {
	c, err := b.getClient()
	if err != nil {
		return nil, err
	}
	p := prof.Params
	if err := c.SetCurrentProfile(p.Profile); err != nil {
		return nil, err
	}
	if err := c.SetReportRate(p.ReportRate); err != nil {
		return nil, err
	}
	if err := c.SetDebounce(p.Debounce); err != nil {
		return nil, err
	}
	if err := c.SetScrollUpTime(p.ScrollUpTime); err != nil {
		return nil, err
	}
	if err := c.SetSleep24G(p.Sleep24G); err != nil {
		return nil, err
	}
	if err := c.SetSleepBT(p.SleepBT); err != nil {
		return nil, err
	}
	if err := c.SetSilentHeight(p.SilentHeight); err != nil {
		return nil, err
	}
	if err := c.SetLineRepair(p.LineRepair); err != nil {
		return nil, err
	}
	if err := c.SetWaveRepair(p.WaveRepair); err != nil {
		return nil, err
	}
	if err := c.SetMotionSync(p.MotionSync); err != nil {
		return nil, err
	}
	if err := c.SetFPS20000(p.FPS20000); err != nil {
		return nil, err
	}
	if err := c.SetSensorAngle(p.AngleTune); err != nil {
		return nil, err
	}
	if err := c.SetLowLatency(p.LowLatency); err != nil {
		return nil, err
	}
	if err := c.SetTrue8K(p.True8K); err != nil {
		return nil, err
	}
	if err := c.SetDPI(p.Profile, prof.DPI); err != nil {
		return nil, err
	}
	if err := c.SetAllKeys(p.Profile, prof.Keys); err != nil {
		return nil, err
	}
	return c.ReadAll()
}
