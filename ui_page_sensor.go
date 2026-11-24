package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

func (a *DriverApp) viewSensorPage(c *ui.Context, p StudioPalette) {
	mp := &a.state.Params

	pageTitle(c, p, "传感器与高级参数",
		"静默高度可视化、传感器握姿倾角微调、移动修正算法、FastTap 低延时、防抖与无线休眠管理",
		nil)

	// Top Row: Visual LOD Visualizer + Visual Sensor Angle Polar Protractor
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		// Left: LOD Lift-Off Distance Card
		studioCard(c, p, "静默高度 (LOD)", "抬起鼠标后传感器完全停止追踪的触发高度", func() {
			lodOptions := a.state.Spec.LODOptions
			if len(lodOptions) == 0 {
				lodOptions = []string{"1 mm", "2 mm"}
			}
			curLabel := lodOptions[0]
			if a.lodIndex >= 0 && a.lodIndex < len(lodOptions) {
				curLabel = lodOptions[a.lodIndex]
			}

			// Custom GPU visualization of mouse elevation above tracking pad
			ui.Box(c).FillWidth().Height(138).Radius(10).
				Background(p.CardElevated).Border(1, p.CardBorder).
				Draw(func(pt *ui.Painter, r ui.Rect) {
					drawLODVisualizer(pt, r, a.lodIndex, len(lodOptions), curLabel, p)
				})

			prevLOD := a.lodIndex
			if ui.Segmented(c.Key("lod-seg"), &a.lodIndex, lodOptions...).
				FillEqually().Label("静默高度").Disabled(a.busy).Changed() {
				lvl := uint8(a.lodIndex)
				lbl := lodOptions[a.lodIndex]
				a.runOp(func() func() {
					err := a.backend.SetSilentHeight(lvl)
					return func() {
						if err != nil {
							a.lodIndex = prevLOD
							a.setToast("设置静默高度失败: "+err.Error(), true)
							return
						}
						if a.state != nil {
							a.state.Params.SilentHeight = lvl
						}
						a.setToast("静默高度已设为 "+lbl, false)
					}
				})
			}
		}).Grow(1)

		// Right: Sensor Angle Tuning Compass (-30° ~ +30°)
		studioCard(c, p, "传感器角度微调", "补偿倾斜握姿导致的自然滑动偏角（-30° 至 +30°）", func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				// GPU Polar Protractor Reticle
				deg := float32(a.angleDraft)
				ui.Box(c).Size(128, 128).Radius(12).
					Background(p.CardElevated).Border(1, p.CardBorder).
					Draw(func(pt *ui.Painter, r ui.Rect) {
						drawAngleCompass(pt, r, deg, p)
					})

				ui.Column(c).Gap(10).Grow(1).Children(func() {
					ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Baseline).Children(func() {
						ui.Text(c, "当前倾角").FontSize(12).TextColor(p.Theme.TextMuted)
						ui.Textf(c, "%+.0f°", a.angleDraft).FontSize(26).Bold().FontFeatures("tnum").TextColor(p.Theme.Accent)
					})
					ui.StepSlider(c.Key("angle-slider"), &a.angleDraft, -30, 30, 1).Label("传感器角度")
					ui.Row(c).Gap(5).Wrap().Children(func() {
						for _, d := range []int8{-30, -15, -10, 0, 10, 15, 30} {
							d := d
							lbl := fmt.Sprintf("%+d°", d)
							if d == 0 {
								lbl = "0°"
							}
							if ui.Button(c.Key(fmt.Sprintf("ang-%d", d)), lbl).Padding(3, 8).Disabled(a.busy).Clicked() {
								a.angleDraft = float64(d)
								a.applyAngle(d)
							}
						}
					})
					ui.Row(c).Justify(ui.End).Children(func() {
						if ui.PrimaryButton(c.Key("apply-angle"), "应用角度").Padding(5, 14).Disabled(a.busy).Clicked() {
							a.applyAngle(int8(math.Round(a.angleDraft)))
						}
					})
				})
			})
		}).Grow(1)
	})

	// Middle Row: Sensor Algorithms & FastTap
	studioCard(c, p, "追踪修正与按键响应引擎", "所有开关修改后均实时写入鼠标板载闪存", func() {
		ui.Column(c).Gap(12).Children(func() {
			a.toggleSettingRow(c, p, "motion-sync", "Motion Sync 运动同步",
				"将传感器 SPI 采样严格对齐 USB/无线回报周期，极大减少微抖动与帧间不一致", &mp.MotionSync,
				func(on bool) error { return a.backend.SetMotionSync(on) })
			ui.Divider(c)
			a.toggleSettingRow(c, p, "line-repair", "直线修正 (Angle Snapping)",
				"自动吸附接近水平或垂直方向的轨迹偏移，适合绘图与特定准星拉枪习惯", &mp.LineRepair,
				func(on bool) error { return a.backend.SetLineRepair(on) })
			ui.Divider(c)
			a.toggleSettingRow(c, p, "wave-repair", "波纹修正 (Ripple Control)",
				"高 DPI 快速移动时通过算法抑制高频波纹抖动，提升指针轨迹顺滑度", &mp.WaveRepair,
				func(on bool) error { return a.backend.SetWaveRepair(on) })
			if a.state.SupportsLowLatency() {
				ui.Divider(c)
				a.toggleSettingRow(c, p, "fast-tap", "FastTap 极限按键响应",
					"大幅降低主按键点击触发延迟（需固件 > v0809），专为 FPS 点射与急停设计", &mp.LowLatency,
					func(on bool) error { return a.backend.SetLowLatency(on) })
			}
		})
	})

	// Bottom Row: Debounce & Sleep Management
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		studioCard(c, p, "微动防抖调校", "平衡触发极速与防双击稳定性", func() {
			ui.Column(c).Gap(14).Children(func() {
				a.numericApplyRow(c, p, "debounce", "按键防抖延迟 (ms)",
					"范围 0 ~ 10 ms（值越小触发越快，默认 3 ms）", &a.debounceDraft, 0, 10, 1,
					func(v float64) {
						ms := uint8(math.Round(v))
						a.runOp(func() func() {
							err := a.backend.SetDebounce(ms)
							return func() {
								if err != nil {
									a.setToast("设置防抖失败: "+err.Error(), true)
									return
								}
								if a.state != nil {
									a.state.Params.Debounce = ms
								}
								a.setToast(fmt.Sprintf("按键防抖已设为 %d ms", ms), false)
							}
						})
					})
				ui.Divider(c)
				a.numericApplyRow(c, p, "scroll-up", "滚轮抬起防抖 (ms)",
					"范围 0 ~ 20 ms，防止滚轮中键回弹误触发", &a.scrollUpDraft, 0, 20, 1,
					func(v float64) {
						ms := uint8(math.Round(v))
						a.runOp(func() func() {
							err := a.backend.SetScrollUpTime(ms)
							return func() {
								if err != nil {
									a.setToast("设置滚轮防抖失败: "+err.Error(), true)
									return
								}
								if a.state != nil {
									a.state.Params.ScrollUpTime = ms
								}
								a.setToast(fmt.Sprintf("滚轮抬起防抖已设为 %d ms", ms), false)
							}
						})
					})
			})
		}).Grow(1)

		studioCard(c, p, "无线休眠策略", "无操作后自动进入低功耗休眠，轻移即可唤醒", func() {
			ui.Column(c).Gap(14).Children(func() {
				// 2.4G Sleep
				ui.Column(c).Gap(6).Children(func() {
					ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Gap(1).Children(func() {
							ui.Text(c, "2.4G 无线休眠").FontSize(13).Bold()
							ui.Text(c, "当前："+formatSleepDuration(mp.Sleep24G)).FontSize(11).TextColor(p.Theme.TextMuted)
						})
						if ui.Switch(c.Key("sleep-24g-sw"), &a.sleep24GOn).Label("启用 2.4G 休眠").Disabled(a.busy).Changed() {
							sec := uint16(0)
							if a.sleep24GOn {
								if a.sleep24GDraft <= 0 {
									a.sleep24GDraft = 120
								}
								sec = uint16(math.Round(a.sleep24GDraft))
							}
							a.applySleep24G(sec)
						}
					})
					if a.sleep24GOn {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.NumberInput(c.Key("sleep-24g-input"), &a.sleep24GDraft, 10, 65535, 30).
								Label("2.4G 休眠秒数").Width(120).Disabled(a.busy)
							ui.Text(c, "秒").FontSize(12).TextColor(p.Theme.TextMuted)
							ui.Spacer(c)
							if ui.Button(c.Key("sleep-24g-apply"), "保存").Disabled(a.busy).Clicked() {
								a.applySleep24G(uint16(math.Round(a.sleep24GDraft)))
							}
						})
					}
				})

				ui.Divider(c)

				// BT Sleep
				ui.Column(c).Gap(6).Children(func() {
					ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Gap(1).Children(func() {
							ui.Text(c, "蓝牙 (BLE) 休眠").FontSize(13).Bold()
							ui.Text(c, "当前："+formatSleepDuration(mp.SleepBT)).FontSize(11).TextColor(p.Theme.TextMuted)
						})
						if ui.Switch(c.Key("sleep-bt-sw"), &a.sleepBTOn).Label("启用蓝牙休眠").Disabled(a.busy).Changed() {
							sec := uint16(0)
							if a.sleepBTOn {
								if a.sleepBTDraft <= 0 {
									a.sleepBTDraft = 120
								}
								sec = uint16(math.Round(a.sleepBTDraft))
							}
							a.applySleepBT(sec)
						}
					})
					if a.sleepBTOn {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.NumberInput(c.Key("sleep-bt-input"), &a.sleepBTDraft, 10, 65535, 30).
								Label("蓝牙休眠秒数").Width(120).Disabled(a.busy)
							ui.Text(c, "秒").FontSize(12).TextColor(p.Theme.TextMuted)
							ui.Spacer(c)
							if ui.Button(c.Key("sleep-bt-apply"), "保存").Disabled(a.busy).Clicked() {
								a.applySleepBT(uint16(math.Round(a.sleepBTDraft)))
							}
						})
					}
				})
			})
		}).Grow(1)
	})
}

func (a *DriverApp) applyAngle(deg int8) {
	a.runOp(func() func() {
		err := a.backend.SetSensorAngle(deg)
		return func() {
			if err != nil {
				a.setToast("设置传感器角度失败: "+err.Error(), true)
				return
			}
			if a.state != nil {
				a.state.Params.AngleTune = deg
			}
			a.setToast(fmt.Sprintf("传感器角度已设为 %+d°", deg), false)
		}
	})
}

func (a *DriverApp) applySleep24G(sec uint16) {
	a.runOp(func() func() {
		err := a.backend.SetSleep24G(sec)
		return func() {
			if err != nil {
				a.setToast("设置 2.4G 休眠失败: "+err.Error(), true)
				return
			}
			if a.state != nil {
				a.state.Params.Sleep24G = sec
			}
			a.setToast("2.4G 休眠已设为 "+formatSleepDuration(sec), false)
		}
	})
}

func (a *DriverApp) applySleepBT(sec uint16) {
	a.runOp(func() func() {
		err := a.backend.SetSleepBT(sec)
		return func() {
			if err != nil {
				a.setToast("设置蓝牙休眠失败: "+err.Error(), true)
				return
			}
			if a.state != nil {
				a.state.Params.SleepBT = sec
			}
			a.setToast("蓝牙休眠已设为 "+formatSleepDuration(sec), false)
		}
	})
}

func (a *DriverApp) toggleSettingRow(c *ui.Context, p StudioPalette, key, title, desc string, val *bool, setter func(bool) error) {
	prev := *val
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title).Bold().FontSize(13)
			ui.Text(c, desc).FontSize(11).TextColor(p.Theme.TextMuted)
		})
		if ui.Switch(c.Key(key), val).Label(title).Disabled(a.busy).Changed() {
			target := *val
			a.runOp(func() func() {
				err := setter(target)
				return func() {
					if err != nil {
						*val = prev
						a.setToast(title+" 修改失败: "+err.Error(), true)
						return
					}
					status := "关闭"
					if target {
						status = "开启"
					}
					a.setToast(fmt.Sprintf("%s 已%s", title, status), false)
				}
			})
		}
	})
}

func (a *DriverApp) numericApplyRow(c *ui.Context, p StudioPalette, key, title, desc string, val *float64, lo, hi, step float64, onApply func(float64)) {
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title).Bold().FontSize(13)
			ui.Text(c, desc).FontSize(11).TextColor(p.Theme.TextMuted)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.NumberInput(c.Key(key+"-input"), val, lo, hi, step).Label(title).Width(104).Disabled(a.busy)
			if ui.Button(c.Key(key+"-apply"), "保存").Disabled(a.busy).Clicked() {
				onApply(*val)
			}
		})
	})
}

var lodMouseSilhouette = ui.MustParsePath(
	"M 10 44 L 128 44 C 138 44 139 32 126 26 C 102 15 74 2 50 2 C 22 2 2 18 2 36 C 2 41 5 44 10 44 Z",
)

// drawLODVisualizer draws a sculpted side-profile gaming mouse hovering above a pad with an optical sensor cone.
func drawLODVisualizer(pt *ui.Painter, r ui.Rect, idx, total int, label string, p StudioPalette) {
	padY := r.Y + r.H - 22
	// Mousepad surface line (dashed)
	padPath := (&ui.Path{}).MoveTo(r.X+20, padY).LineTo(r.X+r.W-20, padY)
	pt.StrokePath(padPath.Dash(0, 6, 4), 1.5, p.Theme.TextMuted.Alpha(0.55))

	// Elevation height scales visually with LOD level
	lift := float32(16)
	if total > 1 {
		lift = 14 + float32(idx)*16
	}
	mouseW := float32(138)
	mouseH := float32(44)
	mx := r.X + (r.W-mouseW)/2
	my := padY - lift - mouseH

	// Optical tracking laser cone underneath mouse sensor
	cx := mx + mouseW*0.52
	cone := (&ui.Path{}).
		MoveTo(cx-6, my+mouseH).
		LineTo(cx+6, my+mouseH).
		LineTo(cx+18, padY).
		LineTo(cx-18, padY).
		Close()
	pt.FillPath(cone, p.Theme.Accent.Alpha(0.16))

	// Sculpted aerodynamic gaming mouse side silhouette
	pt.Transform(ui.Translation(mx, my), 0, 0, func() {
		pt.FillPath(lodMouseSilhouette, p.Theme.Accent.Alpha(0.22))
		pt.StrokePath(lodMouseSilhouette, 1.8, p.Theme.Accent)
	})

	// Scroll wheel accent bump near right nose
	wheel := (&ui.Path{}).Circle(mx+mouseW-34, my+mouseH-22, 4.5)
	pt.FillPath(wheel, p.Theme.Accent)

	// Precision vertical measurement caliper on sensor axis
	beam := (&ui.Path{}).MoveTo(cx, my+mouseH).LineTo(cx, padY)
	pt.StrokePath(beam, 2.0, p.Theme.Accent)
	topTick := (&ui.Path{}).MoveTo(cx-10, my+mouseH+1).LineTo(cx+10, my+mouseH+1)
	botTick := (&ui.Path{}).MoveTo(cx-10, padY-1).LineTo(cx+10, padY-1)
	pt.StrokePath(topTick, 1.5, p.Theme.Accent)
	pt.StrokePath(botTick, 1.5, p.Theme.Accent)

	// Measurement text badge
	sp := ui.Span{Text: "LOD " + label, Size: 12, Weight: 700, Color: p.Theme.Accent}
	_, th := pt.MeasureText(0, sp)
	pt.RichText(cx+24, my+mouseH+(lift-th)/2, 0, sp)
}

// drawAngleCompass draws a polar protractor reticle with rotating sensor axis line (-30°..+30°).
func drawAngleCompass(pt *ui.Painter, r ui.Rect, deg float32, p StudioPalette) {
	cx := r.X + r.W/2
	cy := r.Y + r.H/2
	rad := float32(math.Min(float64(r.W), float64(r.H)))/2 - 14

	// Outer reticle circle
	ring := (&ui.Path{}).Circle(cx, cy, rad)
	pt.StrokePath(ring, 1.2, p.CardBorder)

	// Reference crosshair (0 deg)
	refV := (&ui.Path{}).MoveTo(cx, cy-rad).LineTo(cx, cy+rad)
	refH := (&ui.Path{}).MoveTo(cx-rad, cy).LineTo(cx+rad, cy)
	pt.StrokePath(refV.Dash(0, 3, 3), 1, p.SubtleText.Alpha(0.45))
	pt.StrokePath(refH.Dash(0, 3, 3), 1, p.SubtleText.Alpha(0.45))

	// Rotated sensor vector
	theta := float64(deg) * math.Pi / 180.0
	dx := float32(math.Sin(theta)) * rad
	dy := float32(-math.Cos(theta)) * rad

	axis := (&ui.Path{}).MoveTo(cx-dx*0.65, cy-dy*0.65).LineTo(cx+dx, cy+dy)
	pt.StrokePath(axis, 2.8, p.Theme.Accent)

	// Arrowhead tip dot + center pivot
	tip := (&ui.Path{}).Circle(cx+dx, cy+dy, 4.5)
	pt.FillPath(tip, p.Theme.Accent)
	pivot := (&ui.Path{}).Circle(cx, cy, 3.5)
	pt.FillPath(pivot, p.Theme.Text)
}
