package main

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

// Navigation page IDs matching the full Windows Valkyrie Driver workspace.
const (
	pageKeymap   = "keymap"
	pageDPI      = "dpi"
	pagePerf     = "perf"
	pageSensor   = "sensor"
	pageMacro    = "macro"
	pageProfiles = "profiles"
	pageDevice   = "device"
)

var profileLabels = []string{
	"板载 1", "板载 2", "板载 3", "板载 4",
	"板载 5", "板载 6", "板载 7", "板载 8",
}

// DriverApp owns all UI state, local store, and serialized HID command worker.
type DriverApp struct {
	backend Backend
	store   *LocalStore
	win     *mygo.Window

	// Worker serialization
	opCh   chan func()
	stopCh chan struct{}
	closed atomic.Bool
	busy   bool

	// Connection & device snapshot
	connected bool
	state     *hid.MouseState
	lastErr   string

	// Navigation & theme
	activePage string
	themeMode  string // "system", "dark", "light"
	themeIdx   int

	// Onboard profile selector
	profileLabel string

	// Page 1: Keymap editor state
	selectedSlotIdx int // index into hid.ConfigurableSlots (0..7)
	keyBindTab      int // 0=鼠标 1=组合键 2=火力键 3=多媒体 4=宏绑定 5=禁用
	comboCtrl       bool
	comboShift      bool
	comboAlt        bool
	comboWin        bool
	comboKey1       string
	comboKey2       string
	fireTarget      string
	fireInterval    float64
	fireCount       float64
	bindMacroSlot   string
	bindMacroMode   int // 0=repeat_times 1=on_off 2=touch_repeat

	// Page 2: DPI editor state
	dpiDraft         hid.DPIConfig
	dpiStageCount    int // 1..6 active stage count
	selectedStage    int
	dpiSplitXY       bool
	dpiDirty         bool
	dpiXVal          float64
	dpiYVal          float64
	dpiXSliderPos    float64 // piecewise non-linear 0..1000
	dpiYSliderPos    float64 // piecewise non-linear 0..1000
	dpiXSliderHeld   bool
	dpiYSliderHeld   bool
	dpiAutoApplyAt   time.Time
	dpiColor         ui.Color

	// Page 3 & 4: Performance & Sensor state
	lodIndex      int
	true8KIndex   int // 0=均衡8K 1=满血8K
	angleDraft    float64
	debounceDraft float64
	scrollUpDraft float64
	sleep24GOn    bool
	sleepBTOn     bool
	sleep24GDraft float64
	sleepBTDraft  float64

	// Page 5: Macro Studio state
	savedMacros     []SavedMacro
	selectedMacroID string
	macroNameDraft  string
	macroSlotLabel  string
	macroRepeat     float64
	macroEvents     []hid.MacroEvent
	macroAddKind    int // 0=键盘单击 1=鼠标单击 2=鼠标移动 3=延迟
	macroAddKey     string
	macroAddMouse   string
	macroAddDX      float64
	macroAddDY      float64
	macroAddDelay   float64

	// Page 6: Local Profiles state
	savedProfiles      []SavedProfile
	newProfileName     string
	renameProfileID    string
	renameProfileDraft string
	renameModalOpen    bool
	importModalOpen    bool
	importJSONText     string
	exportModalOpen    bool
	exportJSONText     string
	exportTitle        string

	// Page 7: Device, Firmware & Live Polling Tester state
	fwCheck          hid.FirmwareCheckResult
	checkingFW       bool
	confirmResetOpen bool
	pollTimestamps   []time.Time
	pollCurrentHz    int
	pollPeakHz       int
	pollAvgHz        int
	pollHistory      []int

	// Toast queue (consumed on UI thread inside View)
	pendingToast      string
	pendingToastError bool
}

func NewDriverApp(backend Backend, store *LocalStore) *DriverApp {
	app := &DriverApp{
		backend:       backend,
		store:         store,
		opCh:          make(chan func(), 48),
		stopCh:        make(chan struct{}),
		activePage:    pageKeymap,
		themeMode:     "dark",
		themeIdx:      1,
		profileLabel:  profileLabels[0],
		comboKey1:     "C",
		comboKey2:     "无",
		fireTarget:    "鼠标左键",
		fireInterval:  50,
		fireCount:     0,
		bindMacroSlot: "宏槽位 1",
		macroSlotLabel: "宏槽位 1",
		macroRepeat:   1,
		macroAddKey:   "A",
		macroAddMouse: "左键",
		macroAddDelay: 30,
		sleep24GOn:    true,
		sleepBTOn:     true,
		sleep24GDraft: 120,
		sleepBTDraft:  120,
	}
	app.loadLocalData()
	return app
}

func (a *DriverApp) loadLocalData() {
	if a.store == nil {
		return
	}
	if prefs, err := a.store.LoadPreferences(); err == nil {
		a.themeMode = prefs.ThemeMode
		switch a.themeMode {
		case "system":
			a.themeIdx = 0
		case "dark":
			a.themeIdx = 1
		case "light":
			a.themeIdx = 2
		}
	}
	if profs, err := a.store.LoadProfiles(); err == nil {
		a.savedProfiles = profs
	}
	if macros, err := a.store.LoadMacros(); err == nil {
		a.savedMacros = macros
		if len(macros) > 0 && a.selectedMacroID == "" {
			a.selectLocalMacro(macros[0])
		}
	}
}

func (a *DriverApp) AttachWindow(win *mygo.Window) {
	a.win = win
}

// StartBackground launches the serialized HID worker, initial scan, and 30s battery poll.
func (a *DriverApp) StartBackground() {
	go func() {
		for {
			select {
			case <-a.stopCh:
				return
			case fn := <-a.opCh:
				fn()
			}
		}
	}()

	a.ConnectAsync()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.pollBattery()
			}
		}
	}()
}

func (a *DriverApp) Stop() {
	if a.closed.CompareAndSwap(false, true) {
		close(a.stopCh)
	}
}

func (a *DriverApp) runUI(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	fn()
}

func (a *DriverApp) runOp(work func() (onUI func())) {
	if a.win == nil {
		a.busy = true
		done := work()
		a.busy = false
		if done != nil {
			done()
		}
		return
	}
	a.busy = true
	select {
	case a.opCh <- func() {
		done := work()
		a.runUI(func() {
			a.busy = false
			if done != nil {
				done()
			}
		})
	}:
	default:
		a.busy = false
		a.setToast("设备操作队列繁忙，请稍候", true)
	}
}

func (a *DriverApp) pollBattery() {
	if a.win == nil || a.closed.Load() {
		return
	}
	select {
	case a.opCh <- func() {
		batt, err := a.backend.ReadBattery()
		if err != nil || batt < 0 {
			return
		}
		mygo.RunOnMain(func() {
			if a.connected && a.state != nil && a.state.Battery != batt {
				a.state.Battery = batt
				a.win.Invalidate()
			}
		})
	}:
	default:
	}
}

func (a *DriverApp) setToast(msg string, isErr bool) {
	a.pendingToast = msg
	a.pendingToastError = isErr
}

// syncFromState mirrors the latest hardware snapshot into all page editor drafts.
func (a *DriverApp) syncFromState(st *hid.MouseState) {
	a.state = st
	a.connected = st != nil
	a.lastErr = ""
	if st == nil {
		return
	}

	// DPI state
	a.dpiDraft = st.DPI.Clone()
	a.dpiDirty = false
	a.dpiStageCount = len(a.dpiDraft.Stages)
	if a.dpiStageCount <= 0 {
		a.dpiStageCount = 6
	}
	if a.selectedStage < 0 || a.selectedStage >= len(a.dpiDraft.Stages) {
		a.selectedStage = a.dpiDraft.Current
	}
	if a.selectedStage < 0 || a.selectedStage >= len(a.dpiDraft.Stages) {
		a.selectedStage = 0
	}
	a.syncSelectedStageControls()

	// Sensor & power state
	a.lodIndex = int(st.Params.SilentHeight)
	if len(st.Spec.LODOptions) > 0 && a.lodIndex >= len(st.Spec.LODOptions) {
		a.lodIndex = 0
	}
	if st.Params.True8K {
		a.true8KIndex = 1
	} else {
		a.true8KIndex = 0
	}
	a.angleDraft = float64(st.Params.AngleTune)
	a.debounceDraft = float64(st.Params.Debounce)
	a.scrollUpDraft = float64(st.Params.ScrollUpTime)
	a.sleep24GOn = st.Params.Sleep24G > 0
	a.sleepBTOn = st.Params.SleepBT > 0
	if st.Params.Sleep24G > 0 {
		a.sleep24GDraft = float64(st.Params.Sleep24G)
	}
	if st.Params.SleepBT > 0 {
		a.sleepBTDraft = float64(st.Params.SleepBT)
	}

	if int(st.Params.Profile) < len(profileLabels) {
		a.profileLabel = profileLabels[st.Params.Profile]
	} else {
		a.profileLabel = profileLabels[0]
	}

	a.syncSelectedKeyEditor()

	if a.win != nil {
		a.win.SetTitle(fmt.Sprintf("Valkyrie Studio · %s (%s)", st.Spec.Model, st.Info.Mode.Label()))
	}
}

func (a *DriverApp) syncSelectedStageControls() {
	if len(a.dpiDraft.Stages) == 0 {
		return
	}
	if a.selectedStage < 0 || a.selectedStage >= len(a.dpiDraft.Stages) {
		a.selectedStage = 0
	}
	st := a.dpiDraft.Stages[a.selectedStage]
	a.dpiXVal = float64(st.X)
	a.dpiYVal = float64(st.Y)
	a.dpiSplitXY = st.X != st.Y
	a.dpiColor = ui.RGB(st.R, st.G, st.B)
	minDPI, maxDPI := float64(50), float64(26000)
	if a.state != nil && a.state.Spec.MaxDPI > 0 {
		minDPI = float64(a.state.Spec.MinDPI)
		maxDPI = float64(a.state.Spec.MaxDPI)
	}
	a.dpiXSliderPos = dpiToSliderPos(a.dpiXVal, minDPI, maxDPI)
	a.dpiYSliderPos = dpiToSliderPos(a.dpiYVal, minDPI, maxDPI)
}

func (a *DriverApp) syncSelectedKeyEditor() {
	if a.state == nil || a.selectedSlotIdx < 0 || a.selectedSlotIdx >= len(hid.ConfigurableSlots) {
		return
	}
	slot := hid.ConfigurableSlots[a.selectedSlotIdx]
	b := hid.DecodeKeyAction(a.state.Keys[slot.Index])
	switch b.Kind {
	case hid.KeyKindMouse:
		a.keyBindTab = 0
	case hid.KeyKindCombo:
		a.keyBindTab = 1
		a.comboCtrl, a.comboShift, a.comboAlt, a.comboWin = false, false, false, false
		a.comboKey1, a.comboKey2 = "A", "无"
		nonMods := make([]string, 0, 2)
		for _, code := range []uint8{b.Mod, b.Key1, b.Key2} {
			switch code {
			case 0:
			case hid.ModCtrl:
				a.comboCtrl = true
			case hid.ModShift:
				a.comboShift = true
			case hid.ModAlt:
				a.comboAlt = true
			case hid.ModWin:
				a.comboWin = true
			default:
				if name, ok := hid.HIDKeyName(code); ok {
					nonMods = append(nonMods, name)
				}
			}
		}
		if len(nonMods) > 0 {
			a.comboKey1 = nonMods[0]
		}
		if len(nonMods) > 1 {
			a.comboKey2 = nonMods[1]
		}
	case hid.KeyKindFire:
		a.keyBindTab = 2
		a.fireInterval = float64(maxUint8(b.IntervalMs, 1))
		a.fireCount = float64(b.Count)
		switch b.Target {
		case 240, 0:
			a.fireTarget = "鼠标左键"
		case 241:
			a.fireTarget = "鼠标右键"
		case 242:
			a.fireTarget = "鼠标中键"
		default:
			if name, ok := hid.HIDKeyName(b.Target); ok {
				a.fireTarget = name
			} else {
				a.fireTarget = "鼠标左键"
			}
		}
	case hid.KeyKindMedia:
		a.keyBindTab = 3
	case hid.KeyKindMacro:
		a.keyBindTab = 4
		a.bindMacroSlot = fmt.Sprintf("宏槽位 %d", b.MacroSlot+1)
		switch b.MacroMode {
		case hid.MacroModeOnOff:
			a.bindMacroMode = 1
		case hid.MacroModeTouchRepeat:
			a.bindMacroMode = 2
		default:
			a.bindMacroMode = 0
		}
	case hid.KeyKindDisable:
		a.keyBindTab = 5
	}
}

func maxUint8(v, lo uint8) uint8 {
	if v < lo {
		return lo
	}
	return v
}

func (a *DriverApp) ConnectAsync() {
	a.runOp(func() func() {
		st, err := a.backend.Connect()
		return func() {
			if err != nil {
				a.connected = false
				a.state = nil
				a.lastErr = err.Error()
				return
			}
			a.selectedStage = st.DPI.Current
			a.syncFromState(st)
			a.setToast(fmt.Sprintf("已连接 %s (%s)", st.Spec.Model, st.Info.Mode.Label()), false)
		}
	})
}

func (a *DriverApp) RefreshAsync() {
	a.runOp(func() func() {
		st, err := a.backend.Refresh()
		return func() {
			if err != nil {
				a.connected = false
				a.state = nil
				a.lastErr = err.Error()
				a.setToast("刷新失败: "+err.Error(), true)
				return
			}
			a.syncFromState(st)
			a.setToast("已从鼠标同步最新参数", false)
		}
	})
}

func (a *DriverApp) currentActiveDPI() uint16 {
	if a.state == nil || len(a.state.DPI.Stages) == 0 {
		return 0
	}
	idx := a.state.DPI.Current
	if idx < 0 || idx >= len(a.state.DPI.Stages) {
		idx = 0
	}
	return a.state.DPI.Stages[idx].X
}

// View builds the full MyGo Native UI shell.
func (a *DriverApp) View(c *ui.Context) {
	p := resolvePalette(c, a.themeMode)

	// Debounced automatic DPI write (triggered after NumberInput/ColorWell pause)
	if !a.dpiAutoApplyAt.IsZero() && a.connected && a.state != nil && !a.busy {
		now := c.Now()
		if !now.Before(a.dpiAutoApplyAt) {
			a.dpiAutoApplyAt = time.Time{}
			if a.dpiDirty {
				a.applyDraftDPI()
			}
		} else {
			c.After(a.dpiAutoApplyAt.Sub(now))
		}
	}

	// Flush pending toast notifications safely on UI thread
	if a.pendingToast != "" {
		msg := a.pendingToast
		isErr := a.pendingToastError
		a.pendingToast = ""
		a.pendingToastError = false
		toastType := ""
		if isErr {
			toastType = "error"
		}
		c.AddToast(ui.Toast{
			Title:   msg,
			Type:    toastType,
			Timeout: 3200 * time.Millisecond,
		})
	}

	ui.Row(c).Fill().Background(p.Canvas).Children(func() {
		a.viewSidebar(c, p)

		// Main workspace canvas
		ui.Column(c).Grow(1).FillHeight().Background(p.Canvas).Children(func() {
			if !a.connected || a.state == nil {
				a.viewDisconnected(c, p)
			} else {
				ui.Scroll(c.Key("page-scroll-"+a.activePage)).Grow(1).Padding(22, 26).Children(func() {
					ui.Column(c).Gap(16).Children(func() {
						switch a.activePage {
						case pageKeymap:
							a.viewKeymapPage(c, p)
						case pageDPI:
							a.viewDPIPage(c, p)
						case pagePerf:
							a.viewPerfPage(c, p)
						case pageSensor:
							a.viewSensorPage(c, p)
						case pageMacro:
							a.viewMacroPage(c, p)
						case pageProfiles:
							a.viewProfilesPage(c, p)
						case pageDevice:
							a.viewDevicePage(c, p)
						}
					})
				})
			}
		})
	})

	// Modals & Alerts
	a.viewProfileModals(c, p)
	switch ui.AlertDialog(c, &a.confirmResetOpen,
		"确认恢复鼠标出厂设置？",
		"该操作将清空鼠标板载 DPI、回报率、传感器参数与全部 8 个自定义按键映射，且无法撤销。",
		"取消", "确认重置",
	) {
	case 1:
		a.runOp(func() func() {
			st, err := a.backend.FactoryReset()
			return func() {
				if err != nil {
					a.setToast("恢复出厂设置失败: "+err.Error(), true)
					return
				}
				a.selectedStage = st.DPI.Current
				a.syncFromState(st)
				a.setToast("已恢复出厂设置并重新同步", false)
			}
		})
	}
}

func (a *DriverApp) viewSidebar(c *ui.Context, p StudioPalette) {
	ui.Column(c).Width(242).Shrink(0).FillHeight().
		Background(p.Sidebar).
		BorderWidth(0, 1, 0, 0).BorderColor(p.CardBorder).
		Padding(16, 14).Gap(14).Children(func() {

		// Brand Header
		ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(2, 4).Children(func() {
			ui.Box(c).Size(32, 32).Radius(9).
				Gradient(p.Cyan, ui.Hex("#0284C7"), 135).
				Shadow(0, 4, 12, 0, p.Cyan.Alpha(0.32)).
				Center().Children(func() {
				ui.Icon(c, iconBrand).FontSize(18).TextColor(ui.RGB(255, 255, 255))
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "VALKYRIE").FontSize(14).Bold().LetterSpacing(1.8)
				ui.Text(c, "PRECISION STUDIO").FontSize(9).LetterSpacing(1.2).TextColor(p.Theme.Accent)
			})
		})

		// Compact Device Telemetry Card
		ui.Column(c).Padding(12).Gap(10).Radius(12).
			Background(p.CardElevated).Border(1, p.CardBorder).
			InsetShadow(0, 1, 0, 0, p.CardHighlight).Children(func() {
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					dotCol := p.Crimson
					if a.connected {
						dotCol = p.Emerald
					}
					ui.Box(c).Size(8, 8).Radius(4).Background(dotCol).Shadow(0, 0, 8, 0, dotCol.Alpha(0.7))
					name := "未连接设备"
					if a.connected && a.state != nil {
						name = a.state.Spec.Model
					}
					ui.Text(c, name).FontSize(14).Bold().SingleLine()
				})
				if a.connected && a.state != nil {
					statusPill(c, a.state.FirmwareString(), p.Cyan)
				}
			})

			if a.connected && a.state != nil {
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
					ui.Textf(c, "%s · %s", a.state.Spec.Sensor, a.state.Info.Mode.Label()).
						FontSize(11).TextColor(p.Theme.TextMuted).SingleLine()
				})

				if a.state.Battery >= 0 {
					ui.Column(c).Gap(4).Children(func() {
						ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "实时电量").FontSize(11).TextColor(p.Theme.TextMuted)
							ui.Textf(c, "%d%%", a.state.Battery).FontSize(12).Bold().FontFeatures("tnum").
								TextColor(batteryColor(p, a.state.Battery))
						})
						ui.Meter(c, float64(a.state.Battery), 0, 100, &ui.MeterLevels{
							Warning:       25,
							Critical:      12,
							NormalColor:   p.Emerald,
							WarningColor:  p.Amber,
							CriticalColor: p.Crimson,
						}).Height(5).Label("电量")
					})
				}

				// Onboard profile selector
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "配置层").FontSize(11).TextColor(p.Theme.TextMuted)
					prevProf := a.profileLabel
					if ui.Select(c.Key("sidebar-profile"), &a.profileLabel, profileLabels).
						Label("板载配置").Grow(1).Disabled(a.busy).Changed() {
						idx := 0
						for i, l := range profileLabels {
							if l == a.profileLabel {
								idx = i
								break
							}
						}
						a.runOp(func() func() {
							st, err := a.backend.SetCurrentProfile(uint8(idx))
							return func() {
								if err != nil {
									a.profileLabel = prevProf
									a.setToast("切换板载配置失败: "+err.Error(), true)
									return
								}
								a.selectedStage = st.DPI.Current
								a.syncFromState(st)
								a.setToast(fmt.Sprintf("已切换至 %s", profileLabels[idx]), false)
							}
						})
					}
				})
			} else {
				ui.Text(c, "等待鼠标连接或唤醒").FontSize(11).TextColor(p.Theme.TextMuted)
			}
		})

		// Primary Navigation List
		type navEntry struct {
			id    string
			label string
			sub   string
			icon  *ui.SVG
		}
		navItems := []navEntry{
			{pageKeymap, "按键自定义", "8 键热点 / 组合 / 火力", iconMouse},
			{pageDPI, "DPI 设置", "1–6 档位与灯色", iconDPI},
			{pagePerf, "性能预设", "模式 / 回报率 / 续航", iconPerf},
			{pageSensor, "传感器参数", "LOD / 倾角 / 防抖休眠", iconTune},
			{pageMacro, "宏工作室", "动作编排与板载读写", iconMacro},
			{pageProfiles, "配置方案", "本地快照与导入导出", iconLayer},
			{pageDevice, "设备与工具", "固件 / 测速 / 外观", iconGear},
		}

		ui.Column(c).Gap(4).Grow(1).Children(func() {
			for _, item := range navItems {
				item := item
				active := a.activePage == item.id
				btn := ui.ButtonBase(c.Key("nav-"+item.id)).
					Padding(9, 10).Radius(10).FillWidth().Label(item.label)
				if active {
					btn.Background(p.AccentGlow).Border(1, p.Theme.Accent.Alpha(0.35))
				} else if btn.Hovered() {
					btn.Background(p.Theme.SurfaceHover)
				}
				if btn.Clicked() {
					a.activePage = item.id
				}
				btn.Children(func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).FillWidth().Children(func() {
						// Active pill indicator bar
						barCol := ui.Transparent
						if active {
							barCol = p.Theme.Accent
						}
						ui.Box(c).Size(3, 20).Radius(2).Background(barCol)

						icCol := p.Theme.TextMuted
						txtCol := p.Theme.Text
						if active {
							icCol = p.Theme.Accent
							txtCol = p.Theme.Text
						}
						ui.Icon(c, item.icon).FontSize(17).TextColor(icCol)
						ui.Column(c).Gap(1).Grow(1).Children(func() {
							tEl := ui.Text(c, item.label).FontSize(13).TextColor(txtCol).SingleLine()
							if active {
								tEl.Bold()
							}
							ui.Text(c, item.sub).FontSize(10).TextColor(p.SubtleText).SingleLine()
						})
					})
				})
			}
		})

		// Sidebar Footer
		ui.Divider(c)
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).Children(func() {
			pathStr := "hidraw 未连接"
			if a.connected && a.state != nil {
				pathStr = fmt.Sprintf("%s · %d DPI", a.state.Info.Path, a.currentActiveDPI())
			}
			ui.Text(c, pathStr).Font("monospace").FontSize(10).TextColor(p.SubtleText).Grow(1).SingleLine()
			if a.busy {
				ui.Spinner(c)
			}
			if ui.Button(c.Key("sidebar-sync"), "").Label("同步刷新").Tooltip("重新读取鼠标状态").
				Padding(6, 9).Disabled(a.busy).Children(func() {
				ui.Icon(c, iconSync).FontSize(14)
			}).Clicked() {
				if a.connected {
					a.RefreshAsync()
				} else {
					a.ConnectAsync()
				}
			}
		})
	})
}

func batteryColor(p StudioPalette, pct int) ui.Color {
	switch {
	case pct <= 12:
		return p.Crimson
	case pct <= 25:
		return p.Amber
	default:
		return p.Emerald
	}
}

func (a *DriverApp) viewDisconnected(c *ui.Context, p StudioPalette) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Padding(36).Children(func() {
		ui.Column(c).MaxWidth(520).Padding(28).Gap(16).Radius(16).
			Background(p.Card).Border(1, p.CardBorder).
			InsetShadow(0, 1, 0, 0, p.CardHighlight).
			Shadow(0, 16, 40, 0, p.CardShadow).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(44, 44).Radius(12).Background(p.AccentGlow).Center().Children(func() {
					ui.Icon(c, iconMouse).FontSize(22).TextColor(p.Theme.Accent)
				})
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, "等待 Valkyrie 鼠标连接").FontSize(18).Bold()
					ui.Text(c, "支持蓝牙 (BLE) / 2.4G 无线接收器 / USB 有线直连").FontSize(12).TextColor(p.Theme.TextMuted)
				})
			})
			if a.lastErr != "" {
				ui.Row(c).Padding(10, 12).Radius(10).Background(p.Crimson.Alpha(0.12)).Border(1, p.Crimson.Alpha(0.28)).Children(func() {
					ui.Text(c, a.lastErr).FontSize(12).TextColor(p.Crimson)
				})
			}
			ui.Divider(c)
			ui.Column(c).Gap(7).Children(func() {
				ui.Text(c, "连接排查指引").FontSize(12).Bold()
				ui.Text(c, "• 蓝牙模式：确保鼠标开关拨至 BT 并在系统蓝牙保持连接，若已休眠请轻移鼠标唤醒。").FontSize(12).TextColor(p.Theme.TextMuted)
				ui.Text(c, "• 2.4G / USB 模式：插入接收器或数据线后点击下方按钮立即识别。").FontSize(12).TextColor(p.Theme.TextMuted)
				ui.Text(c, "• 权限检查：系统 udev 规则需放行 VID 374A 的 /dev/hidraw* 节点读写权限。").FontSize(12).TextColor(p.Theme.TextMuted)
			})
			ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
				if ui.PrimaryButton(c.Key("reconnect-btn"), "扫描并连接鼠标").Padding(8, 18).Disabled(a.busy).Clicked() {
					a.ConnectAsync()
				}
			})
		})
	})
}

func snapStep(val, lo, hi, step float64) float64 {
	if val < lo {
		val = lo
	}
	if val > hi {
		val = hi
	}
	snapped := lo + math.Round((val-lo)/step)*step
	if snapped < lo {
		snapped = lo
	}
	if snapped > hi {
		snapped = hi
	}
	return snapped
}
