package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

type perfPresetMeta struct {
	Mode      hid.PerformanceMode
	Title     string
	SpecLine  string
	Desc      string
	GradFrom  ui.Color
	GradTo    ui.Color
	AccentCol ui.Color
}

var perfPresets = []perfPresetMeta{
	{
		Mode:      hid.PerfOffice,
		Title:     "办公模式",
		SpecLine:  "500 Hz · 节能扫描",
		Desc:      "超长续航与极低功耗，适合日常办公与移动便携",
		GradFrom:  ui.Hex("#0EA5E9"),
		GradTo:    ui.Hex("#38BDF8"),
		AccentCol: ui.Hex("#38BDF8"),
	},
	{
		Mode:      hid.PerfPerformance,
		Title:     "性能模式",
		SpecLine:  "1000 Hz · 均衡扫描",
		Desc:      "经典 1ms 电竞标准回报率，兼顾精准响应与持久续航",
		GradFrom:  ui.Hex("#059669"),
		GradTo:    ui.Hex("#34D399"),
		AccentCol: ui.Hex("#34D399"),
	},
	{
		Mode:      hid.PerfGaming,
		Title:     "游戏模式",
		SpecLine:  "4000 Hz · 高频响应",
		Desc:      "0.25ms 超低无线延迟，丝滑匹配高刷电竞屏",
		GradFrom:  ui.Hex("#7C3AED"),
		GradTo:    ui.Hex("#A78BFA"),
		AccentCol: ui.Hex("#A78BFA"),
	},
	{
		Mode:      hid.PerfRage,
		Title:     "狂暴模式",
		SpecLine:  "8000 Hz · 20000 FPS",
		Desc:      "0.125ms 满血性能全开，锁定 20K FPS 竞技扫描",
		GradFrom:  ui.Hex("#E11D48"),
		GradTo:    ui.Hex("#FB7185"),
		AccentCol: ui.Hex("#FB7185"),
	},
}

func (a *DriverApp) detectCurrentPerfMode() (hid.PerformanceMode, bool) {
	p := a.state.Params
	for _, meta := range perfPresets {
		hz, fps, err := meta.Mode.Preset()
		if err == nil && p.ReportRate == hz && p.FPS20000 == fps {
			return meta.Mode, true
		}
	}
	return hid.PerfOffice, false
}

func (a *DriverApp) viewPerfPage(c *ui.Context, p StudioPalette) {
	mp := &a.state.Params
	activeMode, matchedPreset := a.detectCurrentPerfMode()
	hoursMap := a.state.Spec.BatteryHours
	if len(hoursMap) == 0 {
		hoursMap = hid.BatteryHoursForSensor(a.state.Spec.Sensor)
	}

	activeAccent := p.Cyan
	activeHours := ""
	if matchedPreset {
		for _, meta := range perfPresets {
			if meta.Mode == activeMode {
				activeAccent = meta.AccentCol
				activeHours = hoursMap[activeMode]
				break
			}
		}
	}

	pageTitle(c, p, "性能预设与回报率",
		"完整复刻 Windows 版四档性能模式、官方续航预估、7 档回报率与 20000 FPS / 真 8K 竞技引擎",
		nil)

	// Top 2-Column Hero: 4 Mode Cards (left) + Glowing Battery Endurance Telemetry (right)
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		// Left: 2x2 Official Performance Modes
		studioCard(c, p, "模式切换", "点击一键写入对应的回报率与传感器扫描模式", func() {
			ui.Column(c).Gap(10).Children(func() {
				for rowIdx := 0; rowIdx < 2; rowIdx++ {
					ui.Row(c).Gap(10).Children(func() {
						for colIdx := 0; colIdx < 2; colIdx++ {
							meta := perfPresets[rowIdx*2+colIdx]
							// Skip gaming/rage if sensor doesn't publish them (e.g., PAW3311)
							_, supported := hoursMap[meta.Mode]
							if !supported && meta.Mode >= hid.PerfGaming {
								continue
							}
							isSelected := matchedPreset && activeMode == meta.Mode

							cardBtn := ui.ButtonBase(c.Key("perf-mode-"+meta.Title)).
								Grow(1).Padding(14).Radius(12).Label(meta.Title).Disabled(a.busy)
							if isSelected {
								cardBtn.Gradient(meta.GradFrom, meta.GradTo, 135).
									Shadow(0, 8, 22, 0, meta.GradFrom.Alpha(0.38))
							} else {
								cardBtn.Background(p.CardElevated).Border(1, p.CardBorder)
								if cardBtn.Hovered() {
									cardBtn.Border(1, meta.AccentCol.Alpha(0.55))
								}
							}
							if cardBtn.Clicked() {
								targetMode := meta.Mode
								targetTitle := meta.Title
								a.runOp(func() func() {
									err := a.backend.ApplyPerformanceMode(targetMode)
									return func() {
										if err != nil {
											a.setToast("切换"+targetTitle+"失败: "+err.Error(), true)
											return
										}
										if hz, fps, perr := targetMode.Preset(); perr == nil && a.state != nil {
											a.state.Params.ReportRate = hz
											a.state.Params.FPS20000 = fps
										}
										a.setToast("已切换至"+targetTitle, false)
									}
								})
							}
							cardBtn.Children(func() {
								titleCol := p.Theme.Text
								subCol := p.Theme.TextMuted
								specCol := meta.AccentCol
								if isSelected {
									titleCol = ui.RGB(255, 255, 255)
									subCol = ui.RGBA(255, 255, 255, 0.85)
									specCol = ui.RGB(255, 255, 255)
								}
								ui.Column(c).Gap(6).FillWidth().Children(func() {
									ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
										ui.Text(c, meta.Title).FontSize(15).Bold().TextColor(titleCol)
										if h := hoursMap[meta.Mode]; h != "" {
											ui.Textf(c, "约 %sh", h).FontSize(11).Bold().TextColor(specCol).FontFeatures("tnum")
										}
									})
									ui.Text(c, meta.SpecLine).Font("monospace").FontSize(11).Bold().TextColor(specCol)
									ui.Text(c, meta.Desc).FontSize(11).TextColor(subCol).MaxLines(2)
								})
							})
						}
					})
				}
			})
		}).Grow(1)

		// Right: Battery Endurance Telemetry with GPU-drawn glowing battery tank
		studioCard(c, p, "续航预估", "基于官方实验室 2.4G 连续使用数据估算", func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Grow(1).Children(func() {
				ui.Column(c).Gap(8).Grow(1).Children(func() {
					if matchedPreset && activeHours != "" {
						ui.Text(c, "满电连续续航预估").FontSize(12).TextColor(p.Theme.TextMuted)
						ui.Row(c).AlignItems(ui.Baseline).Gap(4).Children(func() {
							ui.Text(c, "约").FontSize(15).TextColor(activeAccent)
							ui.Text(c, activeHours).FontSize(34).Bold().FontFeatures("tnum").TextColor(activeAccent)
							ui.Text(c, "h").FontSize(18).Bold().TextColor(activeAccent)
						})
						statusPill(c, activeMode.Label(), activeAccent)
					} else {
						ui.Text(c, "当前状态").FontSize(12).TextColor(p.Theme.TextMuted)
						ui.Text(c, "自定义模式中").FontSize(24).Bold().TextColor(p.Cyan)
						ui.Textf(c, "%d Hz · 20K FPS %v", mp.ReportRate, boolCN(mp.FPS20000)).
							FontSize(12).TextColor(p.Theme.TextMuted)
					}
					if a.state.Battery >= 0 {
						ui.Textf(c, "当前剩余电量：%d%%", a.state.Battery).FontSize(12).Bold().TextColor(p.Theme.Text)
					}
					ui.Text(c, "* 实际续航受连接模式、休眠时长与使用强度影响").FontSize(10).TextColor(p.SubtleText)
				})

				// Custom GPU-painted vertical glowing battery cell
				battPct := a.state.Battery
				if battPct < 0 {
					battPct = 100
				}
				ui.Box(c).Size(78, 152).Draw(func(pt *ui.Painter, r ui.Rect) {
					drawBatteryTank(pt, r, battPct, activeAccent, p)
				})
			})
		}).Width(330).Shrink(0)
	})

	// 7-Step Polling Rate Precision Bar
	studioCard(c, p, "回报扫描率 (Polling Rate)",
		"提高回报率可显著降低指针响应延迟；蓝牙模式受 BLE 协议限制，设置保存在板载内存中于 2.4G / USB 下生效",
		func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				for _, hz := range hid.SupportedReportRates {
					hz := hz
					active := mp.ReportRate == hz
					label := fmt.Sprintf("%d Hz", hz)
					if hz >= 1000 {
						label = fmt.Sprintf("%dK Hz", hz/1000)
					}
					latMs := 1000.0 / float64(hz)
					latStr := fmt.Sprintf("%.2fms", latMs)
					if latMs >= 1 {
						latStr = fmt.Sprintf("%.1fms", latMs)
					} else if latMs < 0.2 {
						latStr = "0.125ms"
					}

					b := ui.ButtonBase(c.Key(fmt.Sprintf("rate-%d", hz))).
						Grow(1).MinWidth(90).Padding(10, 8).Radius(10).
						Label(label).Disabled(a.busy)
					if active {
						b.Background(p.Theme.Accent).Shadow(0, 4, 14, 0, p.Theme.Accent.Alpha(0.35))
					} else {
						b.Background(p.CardElevated).Border(1, p.CardBorder)
						if b.Hovered() {
							b.Border(1, p.Theme.Accent.Alpha(0.5))
						}
					}
					if b.Clicked() && !active {
						target := hz
						a.runOp(func() func() {
							err := a.backend.SetReportRate(target)
							return func() {
								if err != nil {
									a.setToast("设置回报率失败: "+err.Error(), true)
									return
								}
								if a.state != nil {
									a.state.Params.ReportRate = target
								}
								a.setToast(fmt.Sprintf("回报率已设为 %d Hz", target), false)
							}
						})
					}
					b.Children(func() {
						fg := p.Theme.Text
						sub := p.SubtleText
						if active {
							fg = p.Theme.AccentText
							sub = p.Theme.AccentText.Alpha(0.8)
						}
						ui.Column(c).AlignItems(ui.Center).Gap(3).FillWidth().Children(func() {
							ui.Text(c, label).FontSize(13).Bold().TextColor(fg)
							ui.Text(c, latStr).Font("monospace").FontSize(10).TextColor(sub)
						})
					})
				}
			})
		})

	// Pro Sensor Boost Cards (20000 FPS + 2.4G True 8K)
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		studioCard(c, p, "20000 FPS 竞技扫描模式", "锁定传感器高帧率采样，急停与甩枪追踪更锐利（会增加无线功耗）", func() {
			a.toggleSettingRow(c, p, "perf-fps20k", "开启 20000 FPS 竞技模式",
				"狂暴模式会自动启用此开关，也可在此独立控制", &mp.FPS20000,
				func(on bool) error { return a.backend.SetFPS20000(on) })
		}).Grow(1)

		studioCard(c, p, "回报率增强 (2.4G 满血 8K)", "2.4G 无线 8000 Hz 下可切换均衡 8K 与满血 8K 射频增强", func() {
			ui.Column(c).Gap(10).Children(func() {
				prev := a.true8KIndex
				if ui.Segmented(c.Key("true8k-seg"), &a.true8KIndex, "均衡 8000 Hz", "满血真 8000 Hz").
					FillEqually().Label("回报率增强").Disabled(a.busy).Changed() {
					on := a.true8KIndex == 1
					a.runOp(func() func() {
						err := a.backend.SetTrue8K(on)
						return func() {
							if err != nil {
								a.true8KIndex = prev
								a.setToast("设置真 8K 失败: "+err.Error(), true)
								return
							}
							if a.state != nil {
								a.state.Params.True8K = on
							}
							if on {
								a.setToast("已开启满血真 8000 Hz 增强模式", false)
							} else {
								a.setToast("已切换至均衡 8000 Hz 模式", false)
							}
						}
					})
				}
				if a.state.Info.Mode != hid.Mode24G || mp.ReportRate != 8000 {
					ui.Text(c, "提示：射频满血 8K 增强在 2.4G 无线且回报率为 8000 Hz 时发挥完整效果。").
						FontSize(11).TextColor(p.SubtleText)
				}
			})
		}).Grow(1)
	})
}

func boolCN(v bool) string {
	if v {
		return "开"
	}
	return "关"
}

// drawBatteryTank paints a luxury vertical glowing battery capsule on the GPU.
func drawBatteryTank(pt *ui.Painter, r ui.Rect, pct int, accent ui.Color, p StudioPalette) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	// Top terminal cap
	capW := r.W * 0.36
	capRect := ui.Rect{X: r.X + (r.W-capW)/2, Y: r.Y + 2, W: capW, H: 8}
	pt.Fill(capRect, p.CardBorder, 4)

	// Outer glass body
	body := ui.Rect{X: r.X + 4, Y: r.Y + 12, W: r.W - 8, H: r.H - 16}
	pt.Fill(body, p.CardElevated, 14)
	pt.Stroke(body, p.CardBorder, 1.5, 14)

	// Inner liquid charge fill
	innerPad := float32(6)
	inner := ui.Rect{
		X: body.X + innerPad,
		Y: body.Y + innerPad,
		W: body.W - innerPad*2,
		H: body.H - innerPad*2,
	}
	fillH := inner.H * float32(pct) / 100
	if fillH < 10 && pct > 0 {
		fillH = 10
	}
	fillRect := ui.Rect{
		X: inner.X,
		Y: inner.Y + inner.H - fillH,
		W: inner.W,
		H: fillH,
	}
	pt.FillGradient(fillRect, ui.LinearGradient{
		From:  accent,
		To:    accent.Alpha(0.55),
		Angle: 180,
	}, 9)

	// Percentage label centered inside battery body
	lbl := ui.Span{Text: fmt.Sprintf("%d%%", pct), Size: 15, Weight: 700, Color: p.Theme.Text}
	tw, th := pt.MeasureText(0, lbl)
	pt.RichText(body.X+(body.W-tw)/2, body.Y+(body.H-th)/2, 0, lbl)
}
