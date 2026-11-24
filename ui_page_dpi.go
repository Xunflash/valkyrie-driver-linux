package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

var defaultStageRGBs = [][3]uint8{
	{255, 59, 48},  // Red
	{52, 211, 153}, // Emerald
	{56, 189, 248}, // Cyan
	{167, 139, 250},// Violet
	{251, 191, 36}, // Amber
	{244, 114, 182},// Pink
}

var quickDPIPresets = []uint16{400, 800, 1000, 1200, 1600, 2400, 3200, 6400, 12800}

var quickSwatchColors = []ui.Color{
	ui.RGB(255, 45, 85),
	ui.RGB(255, 149, 0),
	ui.RGB(255, 214, 10),
	ui.RGB(48, 209, 88),
	ui.RGB(56, 189, 248),
	ui.RGB(59, 130, 246),
	ui.RGB(168, 85, 247),
	ui.RGB(255, 255, 255),
}

// Piecewise non-linear DPI curve matching official Windows Mouse_DPI设置new ergonomics:
// 0..200 -> [minDPI..800], 200..400 -> [800..1600], 400..600 -> [1600..3200],
// 600..800 -> [3200..6400], 800..1000 -> [6400..maxDPI].
func dpiCurveKnots(minDPI, maxDPI float64) []float64 {
	if minDPI <= 0 {
		minDPI = 50
	}
	if maxDPI < 6400 {
		maxDPI = 26000
	}
	return []float64{minDPI, 800, 1600, 3200, 6400, maxDPI}
}

func dpiToSliderPos(dpi, minDPI, maxDPI float64) float64 {
	knots := dpiCurveKnots(minDPI, maxDPI)
	if dpi <= knots[0] {
		return 0
	}
	last := len(knots) - 1
	if dpi >= knots[last] {
		return 1000
	}
	segW := 1000.0 / float64(last)
	for i := 0; i < last; i++ {
		lo, hi := knots[i], knots[i+1]
		if dpi <= hi {
			t := 0.0
			if hi > lo {
				t = (dpi - lo) / (hi - lo)
			}
			return float64(i)*segW + t*segW
		}
	}
	return 1000
}

func sliderPosToDPI(pos, minDPI, maxDPI, step float64) float64 {
	knots := dpiCurveKnots(minDPI, maxDPI)
	if pos <= 0 {
		return snapStep(knots[0], minDPI, maxDPI, step)
	}
	last := len(knots) - 1
	if pos >= 1000 {
		return snapStep(knots[last], minDPI, maxDPI, step)
	}
	segW := 1000.0 / float64(last)
	idx := int(pos / segW)
	if idx >= last {
		idx = last - 1
	}
	t := (pos - float64(idx)*segW) / segW
	raw := knots[idx] + t*(knots[idx+1]-knots[idx])
	return snapStep(raw, minDPI, maxDPI, step)
}

func (a *DriverApp) viewDPIPage(c *ui.Context, p StudioPalette) {
	if len(a.dpiDraft.Stages) == 0 {
		ui.Text(c, "未读取到 DPI 档位数据").TextColor(p.Theme.TextMuted)
		return
	}
	spec := a.state.Spec
	minDPI := float64(spec.MinDPI)
	maxDPI := float64(spec.MaxDPI)
	stepDPI := float64(spec.DPIStep)
	if stepDPI <= 0 {
		stepDPI = 50
	}
	maxStages := spec.DPIStages
	if maxStages <= 0 || maxStages > 8 {
		maxStages = 6
	}

	pageTitle(c, p, "DPI 与指示灯设置",
		fmt.Sprintf("%s · %d ~ %d DPI（步进 %d）· 点击档位、预设或松开滑块均自动写入鼠标并实时生效",
			spec.Sensor, spec.MinDPI, spec.MaxDPI, spec.DPIStep),
		func() {
			if a.dpiDirty {
				statusPill(c, "待同步", p.Amber)
			} else {
				statusPill(c, "已同步至鼠标", p.Emerald)
			}
			if ui.Button(c.Key("dpi-restore-defaults"), "恢复默认 DPI").Disabled(a.busy).Clicked() {
				a.restoreDefaultDPIDraft()
				a.applyDraftDPI()
			}
			if ui.PrimaryButton(c.Key("dpi-apply"), "应用 DPI 到鼠标").Padding(7, 16).Disabled(a.busy).Clicked() {
				a.dpiAutoApplyAt = time.Time{}
				a.applyDraftDPI()
			}
		})

	// Stage count selector + 6-Stage Visual Deck
	studioCard(c, p, "板载 DPI 档位队列", "点击任意档位卡片即刻切换鼠标当前生效 DPI 与指示灯色", func() {
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Wrap().Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "启用档位数量").FontSize(12).Bold()
				countIdx := a.dpiStageCount - 1
				if countIdx < 0 {
					countIdx = 0
				}
				if countIdx >= maxStages {
					countIdx = maxStages - 1
				}
				labels := make([]string, maxStages)
				for i := 0; i < maxStages; i++ {
					labels[i] = fmt.Sprintf("%d 档", i+1)
				}
				if ui.Segmented(c.Key("dpi-count-seg"), &countIdx, labels...).
					Label("DPI 档位数量").Disabled(a.busy).Changed() {
					a.resizeDPIStages(countIdx + 1)
					a.applyDraftDPI()
				}
			})
			ui.Textf(c, "当前生效：档位 %d · %d DPI", a.dpiDraft.Current+1, a.dpiDraft.Stages[a.dpiDraft.Current].X).
				Font("monospace").FontSize(12).TextColor(p.Theme.Accent)
		})

		// Stage Cards: clicking a card immediately activates & switches to that DPI stage!
		ui.Row(c).Gap(10).Wrap().Children(func() {
			for i := range a.dpiDraft.Stages {
				i := i
				st := a.dpiDraft.Stages[i]
				stageCol := ui.RGB(st.R, st.G, st.B)
				isSelected := i == a.selectedStage
				isActive := i == a.dpiDraft.Current

				cardBg := p.CardElevated
				borderCol := p.CardBorder
				if isSelected {
					cardBg = p.AccentGlow
					borderCol = p.Theme.Accent
				}

				btn := ui.ButtonBase(c.Key(fmt.Sprintf("dpi-stage-%d", i))).
					Width(124).Padding(0).Radius(12).
					Background(cardBg).Border(1.5, borderCol).
					Label(fmt.Sprintf("DPI 档位 %d", i+1)).Disabled(a.busy)
				if isSelected {
					btn.Shadow(0, 6, 18, 0, p.Theme.Accent.Alpha(0.22))
				}
				if btn.Clicked() {
					a.selectedStage = i
					changedActive := a.dpiDraft.Current != i
					a.dpiDraft.Current = i
					a.syncSelectedStageControls()
					if changedActive {
						a.dpiDirty = true
						a.applyDraftDPI()
					}
				}
				btn.Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						ui.Box(c).FillWidth().Height(5).Radius(3).Background(stageCol)
						ui.Column(c).Padding(10, 11).Gap(6).Children(func() {
							ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
								ui.Textf(c, "档位 %d", i+1).FontSize(11).TextColor(p.Theme.TextMuted)
								if isActive {
									statusPill(c, "当前", p.Emerald)
								}
							})
							if st.X == st.Y {
								ui.Textf(c, "%d DPI", st.X).FontSize(16).Bold().FontFeatures("tnum")
							} else {
								ui.Textf(c, "%d/%d", st.X, st.Y).FontSize(14).Bold().FontFeatures("tnum")
							}
							ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Size(8, 8).Radius(4).Background(stageCol)
								ui.Textf(c, "#%02X%02X%02X", st.R, st.G, st.B).
									Font("monospace").FontSize(10).TextColor(p.SubtleText)
							})
						})
					})
				})
			}
		})
	})

	// Hero Precision Stage Editor
	st := a.dpiDraft.Stages[a.selectedStage]
	stageAccent := ui.RGB(st.R, st.G, st.B)
	studioCard(c, p, fmt.Sprintf("精细编辑 · 档位 %d", a.selectedStage+1), "", func() {
		// Hero DPI readout strip
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Wrap().Gap(14).
			Padding(14, 18).Radius(12).Background(p.CardElevated).Border(1, p.CardBorder).Children(func() {
			ui.Row(c).AlignItems(ui.Baseline).Gap(8).Children(func() {
				if !a.dpiSplitXY {
					ui.Textf(c, "%.0f", a.dpiXVal).FontSize(38).Bold().FontFeatures("tnum").TextColor(p.Theme.Accent)
					ui.Text(c, "DPI").FontSize(14).Bold().TextColor(p.Theme.TextMuted)
				} else {
					ui.Textf(c, "X %.0f", a.dpiXVal).FontSize(30).Bold().FontFeatures("tnum").TextColor(p.Theme.Accent)
					ui.Text(c, "/").FontSize(22).TextColor(p.SubtleText)
					ui.Textf(c, "Y %.0f", a.dpiYVal).FontSize(30).Bold().FontFeatures("tnum").TextColor(p.Violet)
					ui.Text(c, "DPI").FontSize(13).Bold().TextColor(p.Theme.TextMuted)
				}
			})

			ui.Row(c).Gap(14).AlignItems(ui.Center).Wrap().Children(func() {
				statusPill(c, fmt.Sprintf("档位 %d 为当前生效档位", a.dpiDraft.Current+1), p.Emerald)

				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "X / Y 轴独立").FontSize(12).TextColor(p.Theme.TextMuted)
					if ui.Switch(c.Key("dpi-split-xy"), &a.dpiSplitXY).Label("X / Y 轴独立调节").Changed() {
						if !a.dpiSplitXY {
							a.dpiYVal = a.dpiXVal
							a.dpiYSliderPos = a.dpiXSliderPos
							a.commitDraftDPI()
							a.applyDraftDPI()
						}
					}
				})
			})
		})

		// X DPI Non-Linear Slider + Exact Number Input
		xLabel := "灵敏度 (DPI)"
		if a.dpiSplitXY {
			xLabel = "X 轴水平 DPI"
		}
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Text(c, xLabel).Bold().FontSize(13)
				if ui.NumberInput(c.Key("dpi-x-input"), &a.dpiXVal, minDPI, maxDPI, stepDPI).
					Label(xLabel).Width(130).Disabled(a.busy).Changed() {
					a.dpiXVal = snapStep(a.dpiXVal, minDPI, maxDPI, stepDPI)
					a.dpiXSliderPos = dpiToSliderPos(a.dpiXVal, minDPI, maxDPI)
					if !a.dpiSplitXY {
						a.dpiYVal = a.dpiXVal
						a.dpiYSliderPos = a.dpiXSliderPos
					}
					a.commitDraftDPI()
					a.scheduleDPIAutoApply(c)
				}
			})
			slX := ui.Slider(c.Key("dpi-x-slider"), &a.dpiXSliderPos, 0, 1000).Label(xLabel + "滑块")
			if slX.Changed() {
				a.dpiXVal = sliderPosToDPI(a.dpiXSliderPos, minDPI, maxDPI, stepDPI)
				if !a.dpiSplitXY {
					a.dpiYVal = a.dpiXVal
					a.dpiYSliderPos = a.dpiXSliderPos
				}
				a.commitDraftDPI()
				if !slX.Pressed() {
					a.applyDraftDPI()
				}
			}
			if a.dpiXSliderHeld && !slX.Pressed() && a.dpiDirty {
				a.applyDraftDPI()
			}
			a.dpiXSliderHeld = slX.Pressed()

			// Non-linear scale ruler labels matching the 5 segments
			ui.Row(c).Justify(ui.SpaceBetween).Padding(0, 4).Children(func() {
				for _, k := range dpiCurveKnots(minDPI, maxDPI) {
					ui.Textf(c, "%.0f", k).Font("monospace").FontSize(10).TextColor(p.SubtleText)
				}
			})
		})

		// Optional Y DPI Non-Linear Slider + Input
		if a.dpiSplitXY {
			ui.Column(c).Gap(6).Children(func() {
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Y 轴垂直 DPI").Bold().FontSize(13)
					if ui.NumberInput(c.Key("dpi-y-input"), &a.dpiYVal, minDPI, maxDPI, stepDPI).
						Label("Y 轴垂直 DPI").Width(130).Disabled(a.busy).Changed() {
						a.dpiYVal = snapStep(a.dpiYVal, minDPI, maxDPI, stepDPI)
						a.dpiYSliderPos = dpiToSliderPos(a.dpiYVal, minDPI, maxDPI)
						a.commitDraftDPI()
						a.scheduleDPIAutoApply(c)
					}
				})
				slY := ui.Slider(c.Key("dpi-y-slider"), &a.dpiYSliderPos, 0, 1000).Label("Y 轴垂直 DPI 滑块")
				if slY.Changed() {
					a.dpiYVal = sliderPosToDPI(a.dpiYSliderPos, minDPI, maxDPI, stepDPI)
					a.commitDraftDPI()
					if !slY.Pressed() {
						a.applyDraftDPI()
					}
				}
				if a.dpiYSliderHeld && !slY.Pressed() && a.dpiDirty {
					a.applyDraftDPI()
				}
				a.dpiYSliderHeld = slY.Pressed()
			})
		}

		// Quick Esports DPI Presets (immediate write!)
		ui.Row(c).Gap(7).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "快捷预设").FontSize(12).TextColor(p.Theme.TextMuted)
			for _, preset := range quickDPIPresets {
				if preset > spec.MaxDPI {
					continue
				}
				preset := preset
				active := uint16(math.Round(a.dpiXVal)) == preset && uint16(math.Round(a.dpiYVal)) == preset
				if active {
					ui.PrimaryButton(c.Key(fmt.Sprintf("dpi-preset-%d", preset)), fmt.Sprintf("%d", preset)).
						Padding(4, 11).Disabled(a.busy)
				} else if ui.Button(c.Key(fmt.Sprintf("dpi-preset-%d", preset)), fmt.Sprintf("%d", preset)).
					Padding(4, 11).Disabled(a.busy).Clicked() {
					a.dpiXVal = float64(preset)
					a.dpiXSliderPos = dpiToSliderPos(a.dpiXVal, minDPI, maxDPI)
					if !a.dpiSplitXY {
						a.dpiYVal = a.dpiXVal
						a.dpiYSliderPos = a.dpiXSliderPos
					}
					a.commitDraftDPI()
					a.applyDraftDPI()
				}
			}
		})

		ui.Divider(c)

		// LED Indicator Color Picker + Quick Swatches (immediate/debounced write!)
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Wrap().Gap(12).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, "档位指示灯色").FontSize(13).Bold()
				if ui.ColorWell(c.Key("dpi-color-well"), &a.dpiColor).Label("档位指示灯颜色").Changed() {
					a.commitDraftDPI()
					a.scheduleDPIAutoApply(c)
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					for i, sw := range quickSwatchColors {
						sw := sw
						b := ui.ButtonBase(c.Key(fmt.Sprintf("swatch-%d", i))).
							Size(22, 22).Radius(11).Background(sw).
							Border(2, p.CardBorder).Label(fmt.Sprintf("色块 %d", i+1)).Disabled(a.busy)
						if a.dpiColor.R == sw.R && a.dpiColor.G == sw.G && a.dpiColor.B == sw.B {
							b.Border(2.5, ui.RGB(255, 255, 255)).Shadow(0, 0, 8, 0, sw.Alpha(0.75))
						}
						if b.Clicked() {
							a.dpiColor = sw
							a.commitDraftDPI()
							a.applyDraftDPI()
						}
					}
				})
				ui.Textf(c, "#%02X%02X%02X", stageAccent.R, stageAccent.G, stageAccent.B).
					Font("monospace").FontSize(12).TextColor(p.Theme.TextMuted)
			})
		})
	})
}

func (a *DriverApp) scheduleDPIAutoApply(c *ui.Context) {
	a.dpiAutoApplyAt = c.Now().Add(350 * time.Millisecond)
	c.After(350 * time.Millisecond)
}

func (a *DriverApp) resizeDPIStages(n int) {
	if n < 1 {
		n = 1
	}
	maxN := a.state.Spec.DPIStages
	if maxN <= 0 || maxN > 8 {
		maxN = 6
	}
	if n > maxN {
		n = maxN
	}
	cur := a.dpiDraft.Stages
	if len(cur) > n {
		a.dpiDraft.Stages = append([]hid.DPIStage(nil), cur[:n]...)
	} else {
		for len(a.dpiDraft.Stages) < n {
			idx := len(a.dpiDraft.Stages)
			var dpi uint16 = 1600
			if idx < len(a.state.Spec.DefaultDPI) {
				dpi = a.state.Spec.DefaultDPI[idx]
			}
			rgb := defaultStageRGBs[idx%len(defaultStageRGBs)]
			a.dpiDraft.Stages = append(a.dpiDraft.Stages, hid.DPIStage{
				X: dpi, Y: dpi, R: rgb[0], G: rgb[1], B: rgb[2],
			})
		}
	}
	a.dpiStageCount = len(a.dpiDraft.Stages)
	a.dpiDraft.Count = a.dpiStageCount
	if a.dpiDraft.Current >= a.dpiStageCount {
		a.dpiDraft.Current = a.dpiStageCount - 1
	}
	if a.selectedStage >= a.dpiStageCount {
		a.selectedStage = a.dpiStageCount - 1
	}
	a.dpiDirty = true
	a.syncSelectedStageControls()
}

func (a *DriverApp) restoreDefaultDPIDraft() {
	defs := a.state.Spec.DefaultDPI
	if len(defs) == 0 {
		defs = []uint16{400, 800, 1200, 1600, 3200, 5200}
	}
	stages := make([]hid.DPIStage, len(defs))
	for i, d := range defs {
		rgb := defaultStageRGBs[i%len(defaultStageRGBs)]
		stages[i] = hid.DPIStage{X: d, Y: d, R: rgb[0], G: rgb[1], B: rgb[2]}
	}
	cur := a.state.Spec.DefaultCurrentDPI
	if cur < 0 || cur >= len(stages) {
		cur = 0
	}
	a.dpiDraft = hid.DPIConfig{Current: cur, Count: len(stages), Stages: stages}
	a.dpiStageCount = len(stages)
	a.selectedStage = cur
	a.dpiDirty = true
	a.syncSelectedStageControls()
}

func (a *DriverApp) commitDraftDPI() {
	if a.selectedStage < 0 || a.selectedStage >= len(a.dpiDraft.Stages) {
		return
	}
	a.dpiDraft.Stages[a.selectedStage] = hid.DPIStage{
		X: uint16(math.Round(a.dpiXVal)),
		Y: uint16(math.Round(a.dpiYVal)),
		R: a.dpiColor.R,
		G: a.dpiColor.G,
		B: a.dpiColor.B,
	}
	a.dpiDirty = true
}

func (a *DriverApp) applyDraftDPI() {
	a.dpiAutoApplyAt = time.Time{}
	cfg := a.dpiDraft.Clone()
	cfg.Count = len(cfg.Stages)
	prof := a.state.Params.Profile
	a.runOp(func() func() {
		err := a.backend.SetDPI(prof, cfg)
		return func() {
			if err != nil {
				a.setToast("写入 DPI 失败: "+err.Error(), true)
				return
			}
			if a.state != nil {
				a.state.DPI = cfg.Clone()
			}
			a.dpiDirty = false
			st := cfg.Stages[cfg.Current]
			if st.X == st.Y {
				a.setToast(fmt.Sprintf("已切换至档位 %d · %d DPI", cfg.Current+1, st.X), false)
			} else {
				a.setToast(fmt.Sprintf("已切换至档位 %d · X:%d / Y:%d DPI", cfg.Current+1, st.X, st.Y), false)
			}
		}
	})
}
