package main

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *DriverApp) viewDevicePage(c *ui.Context, p StudioPalette) {
	st := a.state

	pageTitle(c, p, "设备信息、回报率测试与设置",
		"硬件实时诊断、在线官方固件校验、实时回报率测速仪、外观主题切换与出厂重置",
		func() {
			if ui.Button(c.Key("dev-sync-btn"), "重新读取设备配置").Disabled(a.busy).Clicked() {
				a.RefreshAsync()
			}
		})

	// Top 2-Column: Hardware Diagnostics + Firmware Check
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		studioCard(c, p, "硬件诊断参数", "", func() {
			ui.Column(c).Gap(5).Children(func() {
				studioInfoRow(c, p, "设备型号", fmt.Sprintf("%s (%s)", st.Spec.Model, st.Spec.Sensor))
				studioInfoRow(c, p, "RongYuan 设备 ID", fmt.Sprintf("%d (0x%04X)", st.DevID, st.DevID))
				studioInfoRow(c, p, "连接方式", st.Info.Mode.Label())
				studioInfoRow(c, p, "VID : PID", st.Info.VIDPID())
				studioInfoRow(c, p, "Linux HID 节点", st.Info.Path)
				studioInfoRow(c, p, "当前固件版本", st.FirmwareString())
				if st.Battery >= 0 {
					studioInfoRow(c, p, "当前电量", fmt.Sprintf("%d%%", st.Battery))
				}
				studioInfoRow(c, p, "DPI 硬件范围", fmt.Sprintf("%d ~ %d DPI（步进 %d）", st.Spec.MinDPI, st.Spec.MaxDPI, st.Spec.DPIStep))
			})
		}).Grow(1)

		ui.Column(c).Grow(1).Gap(16).Children(func() {
			studioCard(c, p, "官方固件在线检查", "连接 RongYuan 官方云端接口校验最新版本", func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(12).Children(func() {
						ui.Column(c).Gap(4).Grow(1).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "当前固件").FontSize(12).TextColor(p.Theme.TextMuted)
								statusPill(c, st.FirmwareString(), p.Cyan)
							})
							if a.fwCheck.Checked {
								col := p.Emerald
								if a.fwCheck.Err != "" {
									col = p.Crimson
								} else if !a.fwCheck.UpToDate {
									col = p.Amber
								}
								ui.Text(c, a.fwCheck.Message).FontSize(12).Bold().TextColor(col)
							} else {
								ui.Text(c, "点击按钮检查是否有新版本固件发布").FontSize(12).TextColor(p.Theme.TextMuted)
							}
						})
						if ui.PrimaryButton(c.Key("check-fw-btn"), "检查最新固件").
							Disabled(a.checkingFW || a.busy).Clicked() {
							a.checkingFW = true
							devID := st.DevID
							curFW := st.FirmwareVer
							a.runOp(func() func() {
								ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
								res := a.backend.CheckFirmware(ctx, devID, curFW)
								cancel()
								return func() {
									a.checkingFW = false
									a.fwCheck = res
									if res.Err != "" {
										a.setToast("固件检查失败: "+res.Message, true)
									} else {
										a.setToast(res.Message, false)
									}
								}
							})
						}
					})
					ui.Text(c, "说明：蓝牙模式仅支持参数读写，刷写新固件需通过 USB 有线连接或 2.4G 接收器进入 Bootloader。").
						FontSize(11).TextColor(p.SubtleText)
				})
			})

			studioCard(c, p, "界面外观主题", "偏好设置将自动保存在 ~/.config/valkyrie-driver/preferences.json", func() {
				if ui.Segmented(c.Key("theme-mode-seg"), &a.themeIdx, "跟随系统", "深色工作室", "浅色瓷白").
					FillEqually().Label("界面外观主题").Changed() {
					modes := []string{"system", "dark", "light"}
					a.themeMode = modes[a.themeIdx]
					if a.store != nil {
						_ = a.store.SavePreferences(AppPreferences{ThemeMode: a.themeMode})
					}
					a.setToast("已切换界面主题", false)
				}
			})
		})
	})

	// Live Mouse Polling Rate Tester Arena (powered by MyGo HandleInput + GPU Painter!)
	studioCard(c, p, "鼠标实时回报率测速仪", "在下方测试区域内快速画圈移动鼠标，实时观测当前 / 平均 / 峰值 Hz 与波形曲线", func() {
		ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
			// Interactive capture arena
			arena := ui.Box(c.Key("poll-arena")).Grow(1).Height(132).Radius(12).
				Background(p.CardElevated).Border(1, p.Theme.Accent.Alpha(0.32)).
				HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputPointerMove {
						a.recordPollSample(ev.Time)
						return true
					}
					return false
				})
			arena.Draw(func(pt *ui.Painter, r ui.Rect) {
				drawPollingWaveform(pt, r, a.pollHistory, p)
			})

			// Telemetry counters
			ui.Column(c).Width(200).Shrink(0).Justify(ui.SpaceBetween).Gap(8).Children(func() {
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, "实时").FontSize(12).TextColor(p.Theme.TextMuted)
					ui.Textf(c, "%d Hz", a.pollCurrentHz).FontSize(22).Bold().FontFeatures("tnum").TextColor(p.Theme.Accent)
				})
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, "平均").FontSize(12).TextColor(p.Theme.TextMuted)
					ui.Textf(c, "%d Hz", a.pollAvgHz).FontSize(16).Bold().FontFeatures("tnum").TextColor(p.Emerald)
				})
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Baseline).Children(func() {
					ui.Text(c, "峰值").FontSize(12).TextColor(p.Theme.TextMuted)
					ui.Textf(c, "%d Hz", a.pollPeakHz).FontSize(16).Bold().FontFeatures("tnum").TextColor(p.Violet)
				})
				if ui.Button(c.Key("poll-reset-btn"), "清零测速数据").FillWidth().Clicked() {
					a.pollTimestamps = nil
					a.pollCurrentHz = 0
					a.pollPeakHz = 0
					a.pollAvgHz = 0
					a.pollHistory = nil
				}
			})
		})
	})

	// Danger Zone: Factory Reset
	studioCard(c, p, "危险操作区", "", func() {
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, "恢复鼠标出厂设置").FontSize(14).Bold().TextColor(p.Crimson)
				ui.Text(c, "将清空鼠标板载 DPI、回报率、传感器参数以及全部自定义按键映射并重置为出厂默认值").
					FontSize(12).TextColor(p.Theme.TextMuted)
			})
			if ui.Button(c.Key("factory-reset-btn"), "恢复出厂设置").
				TextColor(p.Crimson).Border(1, p.Crimson.Alpha(0.5)).
				Padding(7, 16).Disabled(a.busy).Clicked() {
				a.confirmResetOpen = true
			}
		})
	})
}

func (a *DriverApp) recordPollSample(ts time.Time) {
	if ts.IsZero() {
		ts = time.Now()
	}
	cutoff := ts.Add(-time.Second)
	a.pollTimestamps = append(a.pollTimestamps, ts)
	// Prune older than 1s window
	keep := 0
	for keep < len(a.pollTimestamps) && a.pollTimestamps[keep].Before(cutoff) {
		keep++
	}
	if keep > 0 {
		a.pollTimestamps = a.pollTimestamps[keep:]
	}
	hz := len(a.pollTimestamps)
	a.pollCurrentHz = hz
	if hz > a.pollPeakHz {
		a.pollPeakHz = hz
	}
	a.pollHistory = append(a.pollHistory, hz)
	if len(a.pollHistory) > 80 {
		a.pollHistory = a.pollHistory[len(a.pollHistory)-80:]
	}
	sum := 0
	for _, v := range a.pollHistory {
		sum += v
	}
	if len(a.pollHistory) > 0 {
		a.pollAvgHz = sum / len(a.pollHistory)
	}
}

func drawPollingWaveform(pt *ui.Painter, r ui.Rect, history []int, p StudioPalette) {
	// Horizontal subtle grid lines
	for i := 1; i <= 3; i++ {
		y := r.Y + r.H*float32(i)/4
		grid := (&ui.Path{}).MoveTo(r.X+10, y).LineTo(r.X+r.W-10, y)
		pt.StrokePath(grid.Dash(0, 4, 4), 1, p.CardBorder)
	}
	if len(history) < 2 {
		lbl := ui.Span{Text: "在此区域内快速画圈移动鼠标开始测速", Size: 12, Color: p.Theme.TextMuted}
		tw, th := pt.MeasureText(0, lbl)
		pt.RichText(r.X+(r.W-tw)/2, r.Y+(r.H-th)/2, 0, lbl)
		return
	}
	maxVal := 250.0
	for _, v := range history {
		if float64(v) > maxVal {
			maxVal = float64(v)
		}
	}
	maxVal = math.Ceil(maxVal/125.0) * 125.0

	var wave ui.Path
	padX := float32(12)
	padY := float32(14)
	w := r.W - padX*2
	h := r.H - padY*2
	for i, v := range history {
		x := r.X + padX + w*float32(i)/float32(len(history)-1)
		norm := float32(float64(v) / maxVal)
		y := r.Y + padY + h*(1-norm)
		if i == 0 {
			wave.MoveTo(x, y)
		} else {
			wave.LineTo(x, y)
		}
	}
	pt.StrokePath(&wave, 2.2, p.Theme.Accent)
}
