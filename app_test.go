package main

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

type fakeBackend struct {
	state           *hid.MouseState
	connectErr      error
	appliedDPI      *hid.DPIConfig
	appliedRate     int
	appliedLOD      *uint8
	appliedMotion   *bool
	appliedFastTap  *bool
	appliedAngle    *int8
	appliedKeyIdx   *uint8
	appliedKeyCode  [4]byte
	appliedTrue8K   *bool
	appliedPerfMode *hid.PerformanceMode
	appliedAllKeys  *[16][4]byte
	macros          map[uint8]hid.Macro
	writtenMacro    *hid.Macro
	appliedSnapshot *SavedProfile
	factoryResetHit bool
	fwChecked       bool
}

func newFakeConnectedBackend() *fakeBackend {
	st := &hid.MouseState{
		Info: hid.DeviceInfo{
			Path: "/dev/hidraw5",
			VID:  hid.VIDValkyrie,
			PID:  hid.PIDVKM5BT,
			Name: "VK M5",
			Mode: hid.ModeBT,
		},
		Spec:        hid.LookupModel(2914, "VK M5"),
		DevID:       2914,
		FirmwareVer: 0x0810,
		Battery:     87,
		Params: hid.MouseParams{
			Profile:      0,
			Debounce:     3,
			ScrollUpTime: 3,
			Sleep24G:     120,
			SleepBT:      120,
			ReportRate:   1000,
			SilentHeight: 0,
			MotionSync:   true,
			LowLatency:   true,
		},
		DPI: hid.DPIConfig{
			Current: 2,
			Count:   6,
			Stages: []hid.DPIStage{
				{X: 400, Y: 400, R: 255, G: 0, B: 0},
				{X: 800, Y: 800, R: 0, G: 255, B: 0},
				{X: 1200, Y: 1200, R: 0, G: 196, B: 255},
				{X: 1600, Y: 1600, R: 167, G: 139, B: 250},
				{X: 3200, Y: 3200, R: 251, G: 191, B: 36},
				{X: 5200, Y: 5200, R: 251, G: 113, B: 133},
			},
		},
		Keys: hid.DefaultMatrix(),
	}
	return &fakeBackend{
		state: st,
		macros: map[uint8]hid.Macro{
			0: {
				Name:        "快速连招",
				Slot:        0,
				RepeatCount: 2,
				Events: []hid.MacroEvent{
					{Type: hid.MacroKeyboard, Action: hid.MacroActionDown, KeyCode: 4},
					{Type: hid.MacroDelay, DelayMs: 25},
					{Type: hid.MacroKeyboard, Action: hid.MacroActionUp, KeyCode: 4},
				},
			},
		},
	}
}

func (f *fakeBackend) Connect() (*hid.MouseState, error) {
	if f.connectErr != nil {
		return nil, f.connectErr
	}
	cp := *f.state
	cp.DPI = f.state.DPI.Clone()
	return &cp, nil
}
func (f *fakeBackend) Refresh() (*hid.MouseState, error) { return f.Connect() }
func (f *fakeBackend) ReadBattery() (int, error)         { return f.state.Battery, nil }
func (f *fakeBackend) Close() error                      { return nil }
func (f *fakeBackend) SetDPI(profile uint8, cfg hid.DPIConfig) error {
	c := cfg.Clone()
	f.appliedDPI = &c
	f.state.DPI = c.Clone()
	return nil
}
func (f *fakeBackend) SetReportRate(hz int) error {
	f.appliedRate = hz
	f.state.Params.ReportRate = hz
	return nil
}
func (f *fakeBackend) SetSilentHeight(level uint8) error {
	f.appliedLOD = &level
	f.state.Params.SilentHeight = level
	return nil
}
func (f *fakeBackend) SetLineRepair(on bool) error { f.state.Params.LineRepair = on; return nil }
func (f *fakeBackend) SetWaveRepair(on bool) error { f.state.Params.WaveRepair = on; return nil }
func (f *fakeBackend) SetMotionSync(on bool) error {
	f.appliedMotion = &on
	f.state.Params.MotionSync = on
	return nil
}
func (f *fakeBackend) SetFPS20000(on bool) error { f.state.Params.FPS20000 = on; return nil }
func (f *fakeBackend) SetLowLatency(on bool) error {
	f.appliedFastTap = &on
	f.state.Params.LowLatency = on
	return nil
}
func (f *fakeBackend) SetSensorAngle(deg int8) error {
	f.appliedAngle = &deg
	f.state.Params.AngleTune = deg
	return nil
}
func (f *fakeBackend) SetDebounce(ms uint8) error      { f.state.Params.Debounce = ms; return nil }
func (f *fakeBackend) SetScrollUpTime(ms uint8) error  { f.state.Params.ScrollUpTime = ms; return nil }
func (f *fakeBackend) SetSleep24G(sec uint16) error    { f.state.Params.Sleep24G = sec; return nil }
func (f *fakeBackend) SetSleepBT(sec uint16) error     { f.state.Params.SleepBT = sec; return nil }
func (f *fakeBackend) SetCurrentProfile(profile uint8) (*hid.MouseState, error) {
	f.state.Params.Profile = profile
	return f.Connect()
}
func (f *fakeBackend) SetKeySimple(btnIdx uint8, action [4]byte) error {
	f.appliedKeyIdx = &btnIdx
	f.appliedKeyCode = action
	f.state.Keys[btnIdx] = action
	return nil
}
func (f *fakeBackend) ResetDefaultKeys() ([16][4]byte, error) {
	f.state.Keys = hid.DefaultMatrix()
	return f.state.Keys, nil
}
func (f *fakeBackend) FactoryReset() (*hid.MouseState, error) {
	f.factoryResetHit = true
	return f.Connect()
}
func (f *fakeBackend) CheckFirmware(_ context.Context, _ uint32, _ uint16) hid.FirmwareCheckResult {
	f.fwChecked = true
	return hid.FirmwareCheckResult{
		Checked:  true,
		Current:  "v0810",
		Latest:   "v810",
		UpToDate: true,
		Message:  "已是最新官方固件 (v810)",
	}
}
func (f *fakeBackend) SetTrue8K(on bool) error {
	f.appliedTrue8K = &on
	f.state.Params.True8K = on
	return nil
}
func (f *fakeBackend) ApplyPerformanceMode(mode hid.PerformanceMode) error {
	f.appliedPerfMode = &mode
	if hz, fps, err := mode.Preset(); err == nil {
		f.state.Params.ReportRate = hz
		f.state.Params.FPS20000 = fps
	}
	return nil
}
func (f *fakeBackend) SetAllKeys(_ uint8, matrix [16][4]byte) error {
	cp := matrix
	f.appliedAllKeys = &cp
	f.state.Keys = cp
	return nil
}
func (f *fakeBackend) ReadMacro(slot uint8) (hid.Macro, error) {
	if m, ok := f.macros[slot]; ok {
		return m, nil
	}
	return hid.Macro{Slot: slot, RepeatCount: 1}, nil
}
func (f *fakeBackend) WriteMacro(slot uint8, m hid.Macro) error {
	m.Slot = slot
	if f.macros == nil {
		f.macros = make(map[uint8]hid.Macro)
	}
	f.macros[slot] = m
	cp := m
	f.writtenMacro = &cp
	return nil
}
func (f *fakeBackend) ApplyProfileSnapshot(prof SavedProfile) (*hid.MouseState, error) {
	cp := prof
	f.appliedSnapshot = &cp
	f.state.Params = prof.Params
	f.state.DPI = prof.DPI.Clone()
	f.state.Keys = prof.Keys
	return f.Connect()
}

func newTestApp(t *testing.T, fb *fakeBackend) *DriverApp {
	t.Helper()
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	return NewDriverApp(fb, store)
}

func TestUIDisconnectedAndReconnect(t *testing.T) {
	fb := newFakeConnectedBackend()
	fb.connectErr = errors.New("未找到设备")
	app := newTestApp(t, fb)
	app.ConnectAsync()

	tt := ui.NewTester(app.View, 1140, 760)
	if !tt.HasText("等待 Valkyrie 鼠标连接") {
		t.Fatalf("expected disconnected screen, got: %q", tt.Texts())
	}

	fb.connectErr = nil
	if err := tt.Click("扫描并连接鼠标"); err != nil {
		t.Fatalf("click reconnect: %v", err)
	}
	if !app.connected || !tt.HasText("VK M5") || !tt.HasText("87%") {
		t.Fatalf("expected connected UI with VK M5 & 87%%, got connected=%v", app.connected)
	}
}

func TestUIKeymapHotspotsAndAllCategories(t *testing.T) {
	fb := newFakeConnectedBackend()
	app := newTestApp(t, fb)
	app.ConnectAsync()

	tt := ui.NewTester(app.View, 1140, 760)
	// Select Forward side button via its official mouse hotspot pin (#3)
	if err := tt.Click("热点 3 前进侧键"); err != nil {
		t.Fatalf("click hotspot 3: %v", err)
	}
	// Switch to Multimedia category and bind 静音
	if err := tt.Click("多媒体"); err != nil {
		t.Fatalf("click media tab: %v", err)
	}
	if err := tt.Click("静音"); err != nil {
		t.Fatalf("click mute action: %v", err)
	}
	if fb.appliedKeyIdx == nil || *fb.appliedKeyIdx != 3 || fb.appliedKeyCode != [4]byte{3, 0, 226, 0} {
		t.Fatalf("unexpected media binding: idx=%v code=%v", fb.appliedKeyIdx, fb.appliedKeyCode)
	}

	// Test Fire key binding
	if err := tt.Click("火力键"); err != nil {
		t.Fatalf("click fire tab: %v", err)
	}
	if err := tt.Click("写入火力连发键"); err != nil {
		t.Fatalf("click apply fire key: %v", err)
	}
	if fb.appliedKeyCode[0] != 11 || fb.appliedKeyCode[1] != 240 {
		t.Fatalf("expected fire key [11, 240, ...], got %v", fb.appliedKeyCode)
	}
}

func TestUIDPIDynamicStageCountAndApply(t *testing.T) {
	fb := newFakeConnectedBackend()
	app := newTestApp(t, fb)
	app.ConnectAsync()

	tt := ui.NewTester(app.View, 1140, 760)
	if err := tt.Click("DPI 设置"); err != nil {
		t.Fatalf("click DPI nav: %v", err)
	}
	// Clicking stage card 4 immediately activates stage 4 & writes to mouse
	if err := tt.Click("DPI 档位 4"); err != nil {
		t.Fatalf("click stage 4 card: %v", err)
	}
	if fb.appliedDPI == nil || fb.appliedDPI.Current != 3 {
		t.Fatalf("clicking stage 4 did not immediately switch active stage: %+v", fb.appliedDPI)
	}
	// Clicking preset 2400 immediately writes 2400 DPI to active stage 4
	if err := tt.Click("2400"); err != nil {
		t.Fatalf("click 2400 preset: %v", err)
	}
	if fb.appliedDPI == nil || fb.appliedDPI.Stages[3].X != 2400 || fb.appliedDPI.Stages[3].Y != 2400 {
		t.Fatalf("clicking 2400 preset did not immediately write DPI: %+v", fb.appliedDPI)
	}
	// Shrinking stage count to 3 immediately updates mouse stage count & clamps active stage
	if err := tt.Click("3 档"); err != nil {
		t.Fatalf("click 3 stages: %v", err)
	}
	if fb.appliedDPI == nil || len(fb.appliedDPI.Stages) != 3 || fb.appliedDPI.Current != 2 {
		t.Fatalf("unexpected applied DPI after resizing to 3 stages: %+v", fb.appliedDPI)
	}
	// Verify piecewise non-linear slider curve roundtrips accurately
	for _, want := range []float64{50, 400, 800, 1200, 1600, 3200, 6400, 12800, 26000} {
		pos := dpiToSliderPos(want, 50, 26000)
		got := sliderPosToDPI(pos, 50, 26000, 50)
		if got != want {
			t.Fatalf("slider curve roundtrip %.0f -> pos %.2f -> %.0f", want, pos, got)
		}
	}
}

func TestUIPerfSensorMacroProfilesAndDevice(t *testing.T) {
	fb := newFakeConnectedBackend()
	app := newTestApp(t, fb)
	app.ConnectAsync()

	tt := ui.NewTester(app.View, 1140, 860)

	// 1. Performance presets page
	if err := tt.Click("性能预设"); err != nil {
		t.Fatalf("click perf nav: %v", err)
	}
	if err := tt.Click("游戏模式"); err != nil {
		t.Fatalf("click gaming preset: %v", err)
	}
	if fb.appliedPerfMode == nil || *fb.appliedPerfMode != hid.PerfGaming || fb.state.Params.ReportRate != 4000 {
		t.Fatalf("expected PerfGaming 4000Hz, got mode=%v rate=%d", fb.appliedPerfMode, fb.state.Params.ReportRate)
	}
	if !tt.HasText("65-75") {
		t.Fatalf("expected official VK M5 gaming battery hours 65-75")
	}

	// 2. Sensor page
	if err := tt.Click("传感器参数"); err != nil {
		t.Fatalf("click sensor nav: %v", err)
	}
	if err := tt.Click("2 mm"); err != nil {
		t.Fatalf("click 2mm LOD: %v", err)
	}
	if fb.appliedLOD == nil || *fb.appliedLOD != 1 {
		t.Fatalf("expected LOD=1, got %v", fb.appliedLOD)
	}
	if err := tt.Click("-15°"); err != nil {
		t.Fatalf("click -15 deg preset: %v", err)
	}
	if fb.appliedAngle == nil || *fb.appliedAngle != -15 {
		t.Fatalf("expected angle=-15, got %v", fb.appliedAngle)
	}

	// 3. Macro Studio page
	if err := tt.Click("宏工作室"); err != nil {
		t.Fatalf("click macro nav: %v", err)
	}
	if err := tt.Click("从鼠标读取"); err != nil {
		t.Fatalf("click read macro slot: %v", err)
	}
	if len(app.macroEvents) != 3 {
		t.Fatalf("expected 3 macro events loaded, got %d", len(app.macroEvents))
	}
	if err := tt.Click("+ 追加键盘单击"); err != nil {
		t.Fatalf("click append keyboard click: %v", err)
	}
	if err := tt.Click("写入鼠标板载槽位"); err != nil {
		t.Fatalf("click write macro: %v", err)
	}
	if fb.writtenMacro == nil || len(fb.writtenMacro.Events) != 7 {
		t.Fatalf("expected 7 macro events written, got %+v", fb.writtenMacro)
	}

	// 4. Local Profiles page
	if err := tt.Click("配置方案"); err != nil {
		t.Fatalf("click profiles nav: %v", err)
	}
	if err := tt.Click("保存为新方案"); err != nil {
		t.Fatalf("click save profile: %v", err)
	}
	if len(app.savedProfiles) != 1 {
		t.Fatalf("expected 1 saved profile, got %d", len(app.savedProfiles))
	}
	if err := tt.Click("应用到鼠标"); err != nil {
		t.Fatalf("click apply profile snapshot: %v", err)
	}
	if fb.appliedSnapshot == nil {
		t.Fatal("expected ApplyProfileSnapshot call")
	}

	// 5. Device & Tools page
	if err := tt.Click("设备与工具"); err != nil {
		t.Fatalf("click device nav: %v", err)
	}
	if err := tt.Click("检查最新固件"); err != nil {
		t.Fatalf("click check fw: %v", err)
	}
	if !fb.fwChecked || !tt.HasText("已是最新官方固件 (v810)") {
		t.Fatalf("firmware check did not render expected result")
	}
	if err := tt.Click("恢复出厂设置"); err != nil {
		t.Fatalf("click factory reset: %v", err)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("click cancel: %v", err)
	}
	if fb.factoryResetHit {
		t.Fatal("factory reset must not run on Cancel")
	}
	if err := tt.Click("恢复出厂设置"); err != nil {
		t.Fatalf("click factory reset 2nd time: %v", err)
	}
	if err := tt.Click("确认重置"); err != nil {
		t.Fatalf("click confirm reset: %v", err)
	}
	if !fb.factoryResetHit {
		t.Fatal("expected FactoryReset to execute after confirmation")
	}
}

// TestRenderVisualSnapshots renders PNG screenshots of key pages when VK_RENDER_SHOTS=1
// so UI layout, typography, contrast, and alignment can be visually inspected without launching a live window.
func TestRenderVisualSnapshots(t *testing.T) {
	outDir := os.Getenv("VK_RENDER_SHOTS_DIR")
	if outDir == "" {
		t.Skip("set VK_RENDER_SHOTS_DIR to render visual snapshots")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fb := newFakeConnectedBackend()
	app := newTestApp(t, fb)
	app.ConnectAsync()
	// Seed a sample profile & macro so those pages look populated too
	_, _ = app.store.SaveProfile(SavedProfile{
		Name:   "FPS 竞技配置",
		DPI:    fb.state.DPI.Clone(),
		Params: fb.state.Params,
		Keys:   fb.state.Keys,
	})
	app.loadLocalData()

	pages := []string{pageKeymap, pageDPI, pagePerf, pageSensor, pageMacro, pageProfiles, pageDevice}
	for _, pg := range pages {
		app.activePage = pg
		app.themeMode = "dark"
		img := ui.Render(app.View, 1140, 760, 1)
		f, err := os.Create(filepath.Join(outDir, "dark_"+pg+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}
	// Also render light keymap & perf for contrast check
	app.themeMode = "light"
	for _, pg := range []string{pageKeymap, pagePerf, pageSensor} {
		app.activePage = pg
		img := ui.Render(app.View, 1140, 760, 1)
		f, err := os.Create(filepath.Join(outDir, "light_"+pg+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}
}
