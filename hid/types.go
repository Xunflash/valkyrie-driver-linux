package hid

import "fmt"

// ConnectionMode represents how the Valkyrie mouse is connected to Linux.
type ConnectionMode string

const (
	ModeBT  ConnectionMode = "bt"
	ModeUSB ConnectionMode = "usb"
	Mode24G ConnectionMode = "24g"
)

func (m ConnectionMode) Label() string {
	switch m {
	case ModeBT:
		return "蓝牙 (BLE)"
	case ModeUSB:
		return "USB 有线"
	case Mode24G:
		return "2.4G 无线"
	default:
		return string(m)
	}
}

// PerformanceMode is a Valkyrie preset that pairs a polling rate with the 20K FPS sensor flag.
// Values match the official driver (办公 / 性能 / 游戏 / 狂暴).
type PerformanceMode int

const (
	PerfOffice PerformanceMode = iota
	PerfPerformance
	PerfGaming
	PerfRage
)

// Label returns the Chinese preset name shown by the Windows driver.
func (m PerformanceMode) Label() string {
	switch m {
	case PerfOffice:
		return "办公模式"
	case PerfPerformance:
		return "性能模式"
	case PerfGaming:
		return "游戏模式"
	case PerfRage:
		return "狂暴模式"
	default:
		return "未知模式"
	}
}

// Preset returns the polling rate and FPS20000 flag written for this mode.
// Office 500 Hz, Performance 1000 Hz, Gaming 4000 Hz, Rage 8000 Hz + 20K FPS.
func (m PerformanceMode) Preset() (hz int, fps20000 bool, err error) {
	switch m {
	case PerfOffice:
		return 500, false, nil
	case PerfPerformance:
		return 1000, false, nil
	case PerfGaming:
		return 4000, false, nil
	case PerfRage:
		return 8000, true, nil
	default:
		return 0, false, fmt.Errorf("未知性能模式: %d", int(m))
	}
}

// MarshalText encodes the mode as office/performance/gaming/rage for JSON maps.
func (m PerformanceMode) MarshalText() ([]byte, error) {
	switch m {
	case PerfOffice:
		return []byte("office"), nil
	case PerfPerformance:
		return []byte("performance"), nil
	case PerfGaming:
		return []byte("gaming"), nil
	case PerfRage:
		return []byte("rage"), nil
	default:
		return nil, fmt.Errorf("未知性能模式: %d", int(m))
	}
}

// UnmarshalText parses a performance mode name.
func (m *PerformanceMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "office":
		*m = PerfOffice
	case "performance":
		*m = PerfPerformance
	case "gaming":
		*m = PerfGaming
	case "rage":
		*m = PerfRage
	default:
		return fmt.Errorf("未知性能模式: %s", string(text))
	}
	return nil
}

// VK M5 shell used by the Windows driver (RongYuanMouselayout7).
const (
	LayoutRongYuanMouselayout7 = "RongYuanMouselayout7"
	LayoutVKM5Width            = 345
	LayoutVKM5Height           = 509
)

// Known vendor / product IDs for Valkyrie / RongYuan mice.
const (
	VIDValkyrie uint16 = 0x374A
	VIDRongYuan uint16 = 0x3151

	PIDVKM5USB uint16 = 0xA321
	PIDVKM524G uint16 = 0xA322
	PIDVKM5BT  uint16 = 0xA323
)

// ModelSpec describes hardware capabilities for a known RongYuan device ID.
type ModelSpec struct {
	DevID             uint32
	Model             string
	Sensor            string
	MaxDPI            uint16
	MinDPI            uint16
	DPIStep           uint16
	DPIStages         int
	LODOptions        []string
	LowLatencyMin     uint16 // firmware version (decimal-hex as stored in official db, e.g. 0x0809 -> 809)
	DefaultDPI        []uint16
	DefaultCurrentDPI int
	SupportsTrue8K    bool
	BatteryHours      map[PerformanceMode]string
}

func dpiList(v ...uint16) []uint16 {
	return append([]uint16(nil), v...)
}

// batteryHours builds the official续航预估 strings. Empty entries are omitted,
// matching devices that do not publish gaming/rage figures (PAW3311).
func batteryHours(office, performance, gaming, rage string) map[PerformanceMode]string {
	out := make(map[PerformanceMode]string)
	if office != "" {
		out[PerfOffice] = office
	}
	if performance != "" {
		out[PerfPerformance] = performance
	}
	if gaming != "" {
		out[PerfGaming] = gaming
	}
	if rage != "" {
		out[PerfRage] = rage
	}
	return out
}

// BatteryHoursForSensor is the official fallback when a device id is not in the per-id table.
// PAW3395 matches VK M5 (id 2914). PAW3950/PAW3955 use the sensor default (xingneng 80-95).
func BatteryHoursForSensor(sensor string) map[PerformanceMode]string {
	switch sensor {
	case "PAW3311":
		return batteryHours("280-310", "175-200", "", "")
	case "PAW3950", "PAW3955":
		return batteryHours("140-170", "80-95", "35-45", "25-30")
	default:
		return batteryHours("270-300", "160-185", "65-75", "45-60")
	}
}

var knownModels = map[uint32]ModelSpec{
	2914: {
		DevID:             2914,
		Model:             "VK M5",
		Sensor:            "PAW3395",
		MaxDPI:            26000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(400, 800, 1200, 1600, 3200, 5200),
		DefaultCurrentDPI: 2,
		BatteryHours:      batteryHours("270-300", "160-185", "65-75", "45-60"),
	},
	3090: {
		DevID:             3090,
		Model:             "VK M5 Lite",
		Sensor:            "PAW3311",
		MaxDPI:            24000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(800, 1200, 2000, 5000, 8000, 12000),
		DefaultCurrentDPI: 2,
		BatteryHours:      batteryHours("280-310", "175-200", "", ""),
	},
	3395: {
		DevID:             3395,
		Model:             "VK M5 PRO",
		Sensor:            "PAW3950",
		MaxDPI:            42000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"0.7 mm", "1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(400, 800, 1200, 1600, 3200, 5200),
		DefaultCurrentDPI: 2,
		BatteryHours:      BatteryHoursForSensor("PAW3950"),
	},
	// id 3572 is in the official per-id table: xingneng is "88-95", not the PAW3950 sensor fallback.
	3572: {
		DevID:             3572,
		Model:             "VK M5 PRO",
		Sensor:            "PAW3950",
		MaxDPI:            42000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"0.7 mm", "1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(400, 800, 1200, 1600, 3200, 5200),
		DefaultCurrentDPI: 2,
		BatteryHours:      batteryHours("140-170", "88-95", "35-45", "25-30"),
	},
	// id 3855 publishes the PAW3395-class hour table in the official id map.
	3855: {
		DevID:             3855,
		Model:             "VK M5 EVO",
		Sensor:            "PAW3950",
		MaxDPI:            42000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"0.7 mm", "1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(400, 800, 1200, 1600, 3200, 5200),
		DefaultCurrentDPI: 2,
		BatteryHours:      batteryHours("270-300", "160-185", "65-75", "45-60"),
	},
}

// LookupModel returns the hardware spec for devID, falling back to VK M5 defaults.
func LookupModel(devID uint32, fallbackName string) ModelSpec {
	if spec, ok := knownModels[devID]; ok {
		return spec
	}
	name := fallbackName
	if name == "" {
		name = "Valkyrie Mouse"
	}
	return ModelSpec{
		DevID:             devID,
		Model:             name,
		Sensor:            "PAW3395",
		MaxDPI:            26000,
		MinDPI:            50,
		DPIStep:           50,
		DPIStages:         6,
		LODOptions:        []string{"1 mm", "2 mm"},
		LowLatencyMin:     0x0809,
		DefaultDPI:        dpiList(400, 800, 1200, 1600, 3200, 5200),
		DefaultCurrentDPI: 2,
		BatteryHours:      BatteryHoursForSensor("PAW3395"),
	}
}

// DeviceInfo describes a discovered hidraw endpoint.
type DeviceInfo struct {
	Path      string         `json:"path"`
	VID       uint16         `json:"vid"`
	PID       uint16         `json:"pid"`
	Name      string         `json:"name"`
	Mode      ConnectionMode `json:"mode"`
	UsagePage uint16         `json:"usage_page"`
	ReportID  byte           `json:"report_id"`
}

func (d DeviceInfo) VIDPID() string {
	return fmt.Sprintf("%04X:%04X", d.VID, d.PID)
}

// DPIStage represents one DPI stage (X, Y, and LED indicator RGB).
type DPIStage struct {
	X uint16 `json:"x"`
	Y uint16 `json:"y"`
	R uint8  `json:"r"`
	G uint8  `json:"g"`
	B uint8  `json:"b"`
}

// DPIConfig holds all DPI stages and the active stage index for a profile.
type DPIConfig struct {
	Current int        `json:"current"`
	Count   int        `json:"count"`
	Stages  []DPIStage `json:"stages"`
}

// Clone returns a deep copy of DPIConfig.
func (d DPIConfig) Clone() DPIConfig {
	out := d
	out.Stages = append([]DPIStage(nil), d.Stages...)
	return out
}

// MouseParams holds device-wide sensor & power parameters from 0x9F AllParams0.
type MouseParams struct {
	Profile        uint8  `json:"profile"`
	Debounce       uint8  `json:"debounce_ms"`
	ScrollUpTime   uint8  `json:"scroll_up_ms"`
	Sleep24G       uint16 `json:"sleep_24g_sec"`
	SleepBT        uint16 `json:"sleep_bt_sec"`
	ReportRate     int    `json:"report_rate_hz"`
	SilentHeight   uint8  `json:"lod_level"`
	LineRepair     bool   `json:"line_repair"`
	WaveRepair     bool   `json:"wave_repair"`
	MotionSync     bool   `json:"motion_sync"`
	FPS20000       bool   `json:"fps_20000"`
	AngleTune      int8   `json:"angle_tune_deg"`
	LowLatency     bool   `json:"low_latency"`
	True8K         bool   `json:"true_8k"`
	ListReportRate [8]int `json:"-"`
}

// KeySlot describes one configurable physical button on the VK M5.
// HotspotX/HotspotY are official RongYuanMouselayout7 coordinates on a 345x509 shell.
type KeySlot struct {
	Index         uint8
	Name          string
	DefaultAction [4]byte
	HotspotX      float32
	HotspotY      float32
	Order         int
}

// ConfigurableSlots lists the 8 user-remappable VK M5 buttons and wheel directions,
// in official hotspot order (left, right, forward, back, middle, wheel up/down, DPI).
var ConfigurableSlots = []KeySlot{
	{Index: 0, Name: "左键", DefaultAction: [4]byte{1, 0, 240, 0}, HotspotX: 94, HotspotY: 105, Order: 1},
	{Index: 1, Name: "右键", DefaultAction: [4]byte{1, 0, 241, 0}, HotspotX: 210, HotspotY: 105, Order: 2},
	{Index: 3, Name: "前进侧键", DefaultAction: [4]byte{1, 0, 244, 0}, HotspotX: 54, HotspotY: 193, Order: 3},
	{Index: 4, Name: "后退侧键", DefaultAction: [4]byte{1, 0, 243, 0}, HotspotX: 54, HotspotY: 253, Order: 4},
	{Index: 2, Name: "中键 (滚轮点击)", DefaultAction: [4]byte{1, 0, 242, 0}, HotspotX: 155, HotspotY: 110, Order: 5},
	{Index: 14, Name: "滚轮向上", DefaultAction: [4]byte{1, 0, 245, 1}, HotspotX: 155, HotspotY: 78, Order: 6},
	{Index: 15, Name: "滚轮向下", DefaultAction: [4]byte{1, 0, 245, 255}, HotspotX: 155, HotspotY: 140, Order: 7},
	{Index: 5, Name: "DPI 按键", DefaultAction: [4]byte{20, 0, 0, 0}, HotspotX: 172, HotspotY: 330, Order: 8},
}

// DefaultMatrix is the CommonMSCH585 factory 16×4 key matrix from 7237674a.js defaultMatrix.
func DefaultMatrix() [16][4]byte {
	return [16][4]byte{
		{1, 0, 240, 0},
		{1, 0, 241, 0},
		{1, 0, 242, 0},
		{1, 0, 244, 0},
		{1, 0, 243, 0},
		{20, 0, 0, 0},
		{10, 5, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{1, 0, 249, 1},
		{1, 0, 249, 255},
		{1, 0, 245, 1},
		{1, 0, 245, 255},
	}
}

// KeyActionPreset maps a human-readable label to its 4-byte RongYuan matrix code.
type KeyActionPreset struct {
	Label string
	Code  [4]byte
}

// KeyPresets are verified 4-byte action entries from RongYuan CommonMSCH585 (Zo table).
var KeyPresets = []KeyActionPreset{
	{Label: "鼠标左键", Code: [4]byte{1, 0, 240, 0}},
	{Label: "鼠标右键", Code: [4]byte{1, 0, 241, 0}},
	{Label: "鼠标中键", Code: [4]byte{1, 0, 242, 0}},
	{Label: "鼠标前进", Code: [4]byte{1, 0, 244, 0}},
	{Label: "鼠标后退", Code: [4]byte{1, 0, 243, 0}},
	{Label: "DPI 循环切换", Code: [4]byte{20, 0, 0, 0}},
	{Label: "DPI +", Code: [4]byte{20, 0, 1, 0}},
	{Label: "DPI -", Code: [4]byte{0, 2, 0, 20}},
	{Label: "滚轮向上", Code: [4]byte{1, 0, 245, 1}},
	{Label: "滚轮向下", Code: [4]byte{1, 0, 245, 255}},
	{Label: "滚轮向左", Code: [4]byte{1, 0, 245, 0}},
	{Label: "滚轮向右", Code: [4]byte{1, 0, 246, 0}},
	{Label: "复制 (Ctrl+C)", Code: [4]byte{0, 0, 224, 6}},
	{Label: "粘贴 (Ctrl+V)", Code: [4]byte{0, 0, 224, 25}},
	{Label: "剪切 (Ctrl+X)", Code: [4]byte{0, 0, 224, 27}},
	{Label: "显示桌面 (Win+D)", Code: [4]byte{0, 0, 227, 7}},
	{Label: "截屏 (PrintScreen)", Code: [4]byte{0, 0, 70, 0}},
	{Label: "禁用按键", Code: [4]byte{0, 0, 0, 0}},
}

// KeyPresetLabels returns the display label slice for ui.Select.
func KeyPresetLabels() []string {
	out := make([]string, len(KeyPresets))
	for i, p := range KeyPresets {
		out[i] = p.Label
	}
	return out
}

// LabelForKeyCode returns the human label for a 4-byte matrix code.
func LabelForKeyCode(code [4]byte) string {
	for _, p := range KeyPresets {
		if p.Code == code {
			return p.Label
		}
	}
	// Newer firmware stores DPI- as [20,0,2,0]. The legacy preset keeps [0,2,0,20].
	if code == [4]byte{20, 0, 2, 0} {
		return "DPI -"
	}
	return fmt.Sprintf("自定义 [%d,%d,%d,%d]", code[0], code[1], code[2], code[3])
}

// KeyCodeForLabel resolves a preset label back to its 4-byte matrix code.
func KeyCodeForLabel(label string) ([4]byte, bool) {
	for _, p := range KeyPresets {
		if p.Label == label {
			return p.Code, true
		}
	}
	return [4]byte{}, false
}

// MouseState is the complete snapshot of a connected mouse.
type MouseState struct {
	Info        DeviceInfo  `json:"device"`
	Spec        ModelSpec   `json:"spec"`
	DevID       uint32      `json:"dev_id"`
	FirmwareVer uint16      `json:"firmware_ver"`
	Battery     int         `json:"battery_pct"` // -1 if unavailable (e.g. wired USB without battery query)
	Params      MouseParams `json:"params"`
	DPI         DPIConfig   `json:"dpi"`
	Keys        [16][4]byte `json:"keys"`
}

// FirmwareString formats firmware version in the official Valkyrie format (e.g. 0x0810 -> "v0810").
func (s MouseState) FirmwareString() string {
	if s.FirmwareVer == 0 {
		return "未知"
	}
	return fmt.Sprintf("v%04x", s.FirmwareVer)
}

// SupportsLowLatency reports whether the connected firmware supports FastTap low-latency mode.
func (s MouseState) SupportsLowLatency() bool {
	if s.Spec.LowLatencyMin == 0 {
		return s.FirmwareVer > 0x0809
	}
	return s.FirmwareVer > s.Spec.LowLatencyMin
}

// FirmwareCheckResult holds the result of checking RongYuan's online firmware API.
type FirmwareCheckResult struct {
	Checked  bool   `json:"checked"`
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	UpToDate bool   `json:"up_to_date"`
	Message  string `json:"message"`
	Err      string `json:"error,omitempty"`
}

// Transport abstracts BT/USB/2.4G hidraw framing so protocol logic can be unit tested cleanly.
type Transport interface {
	DeviceInfo() DeviceInfo
	// Exchange sends a 64-byte Bit7-encoded command frame and returns the 64-byte response frame.
	Exchange(cmd []byte, expectReply bool) ([]byte, error)
	// ReadBattery queries live battery percentage (0..100, or -1 if unsupported) and optional fw version.
	ReadBattery() (int, uint16, error)
	Close() error
}
