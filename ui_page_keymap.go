package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

var (
	comboKeyChoices    []string
	comboKey2Choices   []string
	fireTargetChoices  []string
	macroSlotChoices   []string
	macroModeChoices   = []string{"循环指定次数", "按键开关循环", "按住持续循环"}
	keyCategoryLabels = []string{"鼠标", "组合键", "火力键", "多媒体", "宏绑定", "禁用"}
)

func init() {
	for _, k := range hid.HIDKeys {
		// Exclude modifier-only keys from primary key dropdowns (modifiers have dedicated toggle pills)
		if k.Code >= hid.ModCtrl && k.Code <= hid.ModWin {
			continue
		}
		comboKeyChoices = append(comboKeyChoices, k.Name)
	}
	comboKey2Choices = append([]string{"无"}, comboKeyChoices...)
	fireTargetChoices = append([]string{"鼠标左键", "鼠标右键", "鼠标中键"}, comboKeyChoices...)
	for i := 1; i <= 16; i++ {
		macroSlotChoices = append(macroSlotChoices, fmt.Sprintf("宏槽位 %d", i))
	}
}

func (a *DriverApp) viewKeymapPage(c *ui.Context, p StudioPalette) {
	if a.selectedSlotIdx < 0 || a.selectedSlotIdx >= len(hid.ConfigurableSlots) {
		a.selectedSlotIdx = 0
	}
	slot := hid.ConfigurableSlots[a.selectedSlotIdx]
	curCode := a.state.Keys[slot.Index]

	pageTitle(c, p, "按键自定义",
		"点击鼠标图上的编号热点选择物理按键，支持鼠标、组合键、火力连发、多媒体与板载宏绑定",
		func() {
			if ui.Button(c.Key("reset-all-keys"), "恢复 8 键默认映射").Disabled(a.busy).Clicked() {
				a.runOp(func() func() {
					keys, err := a.backend.ResetDefaultKeys()
					return func() {
						if err != nil {
							a.setToast("恢复默认按键失败: "+err.Error(), true)
							return
						}
						if a.state != nil {
							a.state.Keys = keys
						}
						a.syncSelectedKeyEditor()
						a.setToast("已恢复全部 8 个按键为出厂默认功能", false)
					}
				})
			}
		})

	ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
		// Left: Interactive VK M5 Mouse Stage with official RongYuanMouselayout7 hotspots
		ui.Column(c).Width(316).Shrink(0).Gap(12).Children(func() {
			studioCard(c, p, "VK M5 物理按键视图", "点击任意编号热点选中按键", func() {
				const stageW float32 = 276
				const imgW float32 = 232
				const imgH float32 = 342
				scaleX := imgW / float32(hid.LayoutVKM5Width)
				scaleY := imgH / float32(hid.LayoutVKM5Height)
				offX := (stageW - imgW) / 2
				const offY float32 = 10

				ui.Box(c).Size(stageW, imgH+20).Radius(12).
					RadialGradient(ui.RadialGradient{
						Center: ui.Point{X: 0.5, Y: 0.42},
						Stops: []ui.GradientStop{
							{Color: p.Theme.Accent.Alpha(0.14), At: 0},
							{Color: p.CardElevated, At: 0.78},
						},
					}).Border(1, p.CardBorder).Children(func() {
					if bmp := getVKM5Bitmap(); bmp != nil {
						ui.Image(c, bmp).Absolute().Left(offX).Top(offY).Size(imgW, imgH).Fit(ui.Contain)
					}

					// Overlay 8 official numbered hotspot pins
					for i, s := range hid.ConfigurableSlots {
						i := i
						s := s
						code := a.state.Keys[s.Index]
						isSel := i == a.selectedSlotIdx
						isDisabled := code == [4]byte{}
						isModified := code != s.DefaultAction

						pinBg := ui.RGBA(30, 41, 59, 0.88)
						pinBorder := ui.RGBA(148, 163, 184, 0.55)
						pinText := ui.RGB(241, 245, 249)
						if isDisabled {
							pinBg = p.Crimson
							pinBorder = ui.RGB(255, 255, 255)
						} else if isModified {
							pinBg = p.Emerald
							pinBorder = ui.RGB(255, 255, 255)
							pinText = ui.Hex("#042016")
						}
						if isSel {
							pinBg = p.Theme.Accent
							pinBorder = ui.RGB(255, 255, 255)
							pinText = p.Theme.AccentText
						}

						const pinSize float32 = 21
						px := offX + s.HotspotX*scaleX - pinSize/2
						py := offY + s.HotspotY*scaleY - pinSize/2
						// Add slight visual separation between the 3 stacked scroll-wheel pins (6/5/7)
						if s.Index == 14 {
							py -= 5
						} else if s.Index == 15 {
							py += 5
						}
						// Clamp inside stage bounds
						if px < 6 {
							px = 6
						}
						if px > stageW-pinSize-6 {
							px = stageW - pinSize - 6
						}
						if py < 6 {
							py = 6
						}
						if py > imgH+14-pinSize {
							py = imgH + 14 - pinSize
						}

						pin := ui.ButtonBase(c.Key(fmt.Sprintf("hotspot-%d", s.Index))).
							Absolute().Left(px).Top(py).Size(pinSize, pinSize).Radius(pinSize/2).
							Background(pinBg).Border(2, pinBorder).Center().
							Label(fmt.Sprintf("热点 %d %s", s.Order, s.Name)).
							Tooltip(fmt.Sprintf("#%d %s · %s", s.Order, s.Name, hid.BindingSummary(code)))
						if isSel {
							pin.Shadow(0, 0, 14, 2, p.Theme.Accent.Alpha(0.75))
						} else {
							pin.Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.45))
						}
						if pin.Clicked() {
							a.selectedSlotIdx = i
							a.syncSelectedKeyEditor()
						}
						pin.Children(func() {
							ui.Textf(c, "%d", s.Order).FontSize(11).Bold().TextColor(pinText)
						})
					}
				})

				// Legend row
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Padding(2, 4).Children(func() {
					legendDot(c, p, p.Theme.Accent, "当前选中")
					legendDot(c, p, p.Emerald, "已自定义")
					legendDot(c, p, p.Crimson, "已禁用")
					legendDot(c, p, ui.Hex("#475569"), "出厂默认")
				})

				ui.Divider(c)

				// Compact clickable list of all 8 slots
				ui.Column(c).Gap(4).Children(func() {
					for i, s := range hid.ConfigurableSlots {
						i := i
						s := s
						code := a.state.Keys[s.Index]
						sel := i == a.selectedSlotIdx
						row := ui.ButtonBase(c.Key(fmt.Sprintf("slot-row-%d", s.Index))).
							Padding(6, 9).Radius(8).FillWidth().Label(s.Name)
						if sel {
							row.Background(p.AccentGlow).Border(1, p.Theme.Accent.Alpha(0.4))
						} else if row.Hovered() {
							row.Background(p.Theme.SurfaceHover)
						}
						if row.Clicked() {
							a.selectedSlotIdx = i
							a.syncSelectedKeyEditor()
						}
						row.Children(func() {
							ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).FillWidth().Gap(8).Children(func() {
								ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
									ui.Textf(c, "#%d", s.Order).Font("monospace").FontSize(11).TextColor(p.Theme.Accent)
									ui.Text(c, s.Name).FontSize(12).SingleLine()
								})
								sumCol := p.Theme.TextMuted
								if code == [4]byte{} {
									sumCol = p.Crimson
								} else if code != s.DefaultAction {
									sumCol = p.Emerald
								}
								ui.Text(c, hid.BindingSummary(code)).FontSize(11).Bold().TextColor(sumCol).SingleLine()
							})
						})
					}
				})
			})
		})

		// Right: Full 6-Category Keybinding Inspector
		ui.Column(c).Grow(1).Gap(14).Children(func() {
			studioCard(c, p, "", "", func() {
				// Selected slot header banner
				ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Wrap().Gap(10).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							statusPill(c, fmt.Sprintf("按键 #%d", slot.Order), p.Theme.Accent)
							ui.Text(c, slot.Name).FontSize(17).Bold()
							statusPill(c, hid.BindingSummary(curCode), p.Emerald)
						})
						ui.Textf(c, "矩阵槽位 %d · 当前指令码 [%d, %d, %d, %d]",
							slot.Index, curCode[0], curCode[1], curCode[2], curCode[3]).
							Font("monospace").FontSize(11).TextColor(p.SubtleText)
					})
					if ui.Button(c.Key("reset-single-key"), "恢复此键默认").
						Disabled(a.busy || curCode == slot.DefaultAction).Clicked() {
						a.applyKeyAction(slot, slot.DefaultAction, "默认功能")
					}
				})

				ui.Divider(c)

				// 6-Category Segmented Switcher
				ui.Segmented(c.Key("key-cat-seg"), &a.keyBindTab, keyCategoryLabels...).
					FillEqually().Label("功能分类")

				switch a.keyBindTab {
				case 0:
					a.viewKeyBindMouse(c, p, slot, curCode)
				case 1:
					a.viewKeyBindCombo(c, p, slot)
				case 2:
					a.viewKeyBindFire(c, p, slot)
				case 3:
					a.viewKeyBindMedia(c, p, slot, curCode)
				case 4:
					a.viewKeyBindMacro(c, p, slot)
				case 5:
					a.viewKeyBindDisable(c, p, slot, curCode)
				}
			})
		})
	})
}

func legendDot(c *ui.Context, p StudioPalette, col ui.Color, label string) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(col)
		ui.Text(c, label).FontSize(10).TextColor(p.Theme.TextMuted)
	})
}

func (a *DriverApp) applyKeyAction(slot hid.KeySlot, code [4]byte, desc string) {
	a.runOp(func() func() {
		err := a.backend.SetKeySimple(slot.Index, code)
		return func() {
			if err != nil {
				a.setToast("按键写入失败: "+err.Error(), true)
				return
			}
			if a.state != nil {
				a.state.Keys[slot.Index] = code
			}
			a.setToast(fmt.Sprintf("已将「%s」设为「%s」", slot.Name, desc), false)
		}
	})
}

func (a *DriverApp) viewKeyBindMouse(c *ui.Context, p StudioPalette, slot hid.KeySlot, curCode [4]byte) {
	actions := hid.MouseActions()
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, "点击下方任意鼠标、滚轮、DPI 或板载配置切换功能即可立即写入鼠标：").
			FontSize(12).TextColor(p.Theme.TextMuted)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, act := range actions {
				act := act
				active := curCode == act.Code || (act.Name == "DPI-" && curCode == [4]byte{0, 2, 0, 20})
				if active {
					ui.PrimaryButton(c.Key("mact-"+act.Name), act.Name).
						Padding(7, 14).Disabled(a.busy)
				} else if ui.Button(c.Key("mact-"+act.Name), act.Name).
					Padding(7, 14).Disabled(a.busy).Clicked() {
					a.applyKeyAction(slot, act.Code, act.Name)
				}
			}
		})
	})
}

func (a *DriverApp) viewKeyBindCombo(c *ui.Context, p StudioPalette, slot hid.KeySlot) {
	ui.Column(c).Gap(14).Children(func() {
		ui.Text(c, "支持最多 3 个键码同时触发（修饰键 + 主键 / 双主键），一键绑定快捷键：").
			FontSize(12).TextColor(p.Theme.TextMuted)

		// Modifier toggles
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "修饰键（可多选，最多占 2 个槽位）").FontSize(12).Bold()
			ui.Row(c).Gap(8).Wrap().Children(func() {
				modTogglePill(c, p, "mod-ctrl", "Ctrl", &a.comboCtrl)
				modTogglePill(c, p, "mod-shift", "Shift", &a.comboShift)
				modTogglePill(c, p, "mod-alt", "Alt", &a.comboAlt)
				modTogglePill(c, p, "mod-win", "Win / Super", &a.comboWin)
			})
		})

		// Primary & Secondary key dropdowns
		ui.Row(c).Gap(14).Wrap().Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "主键").FontSize(12).Bold()
				ui.Select(c.Key("combo-key1"), &a.comboKey1, comboKeyChoices).
					Label("主键").Width(170)
			})
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "副键（可选）").FontSize(12).Bold()
				ui.Select(c.Key("combo-key2"), &a.comboKey2, comboKey2Choices).
					Label("副键").Width(170)
			})
		})

		codes := a.buildComboCodes()
		preview := hid.BindingSummary(hid.ComboKeys(codes...).Code)
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Padding(10, 14).Radius(10).
			Background(p.CardElevated).Border(1, p.CardBorder).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "预览组合：").FontSize(12).TextColor(p.Theme.TextMuted)
				ui.Text(c, preview).FontSize(14).Bold().TextColor(p.Theme.Accent)
			})
			if ui.PrimaryButton(c.Key("apply-combo-btn"), "写入组合键").
				Disabled(a.busy || len(codes) == 0).Clicked() {
				b := hid.ComboKeys(codes...)
				a.applyKeyAction(slot, b.Code, preview)
			}
		})
	})
}

func modTogglePill(c *ui.Context, p StudioPalette, key, label string, val *bool) {
	if *val {
		if ui.PrimaryButton(c.Key(key), label).Padding(6, 14).Clicked() {
			*val = false
		}
	} else {
		if ui.Button(c.Key(key), label).Padding(6, 14).Clicked() {
			*val = true
		}
	}
}

func (a *DriverApp) buildComboCodes() []uint8 {
	var codes []uint8
	if a.comboCtrl {
		codes = append(codes, hid.ModCtrl)
	}
	if a.comboShift {
		codes = append(codes, hid.ModShift)
	}
	if a.comboAlt {
		codes = append(codes, hid.ModAlt)
	}
	if a.comboWin {
		codes = append(codes, hid.ModWin)
	}
	if len(codes) < 3 {
		if kc, ok := hid.HIDKeyCode(a.comboKey1); ok {
			codes = append(codes, kc)
		}
	}
	if len(codes) < 3 && a.comboKey2 != "" && a.comboKey2 != "无" {
		if kc, ok := hid.HIDKeyCode(a.comboKey2); ok {
			codes = append(codes, kc)
		}
	}
	if len(codes) > 3 {
		codes = codes[:3]
	}
	return codes
}

func (a *DriverApp) viewKeyBindFire(c *ui.Context, p StudioPalette, slot hid.KeySlot) {
	ui.Column(c).Gap(14).Children(func() {
		ui.Text(c, "硬件级高速连发键（固件自动执行点击节奏，连发次数设为 0 表示按住持续连发）：").
			FontSize(12).TextColor(p.Theme.TextMuted)

		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "连发目标按键").FontSize(13).Bold().Width(110)
			ui.Select(c.Key("fire-target"), &a.fireTarget, fireTargetChoices).
				Label("连发目标按键").Width(180)
		})

		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "连发间隔 (ms)").FontSize(13).Bold()
				ui.NumberInput(c.Key("fire-int-num"), &a.fireInterval, 1, 255, 5).Label("连发间隔").Width(110)
			})
			ui.StepSlider(c.Key("fire-int-slider"), &a.fireInterval, 1, 255, 1).Label("连发间隔滑块")
		})

		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "连发次数").FontSize(13).Bold()
					if math.Round(a.fireCount) == 0 {
						statusPill(c, "0 = 按住持续连发", p.Cyan)
					}
				})
				ui.NumberInput(c.Key("fire-cnt-num"), &a.fireCount, 0, 255, 1).Label("连发次数").Width(110)
			})
			ui.StepSlider(c.Key("fire-cnt-slider"), &a.fireCount, 0, 255, 1).Label("连发次数滑块")
		})

		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.PrimaryButton(c.Key("apply-fire-btn"), "写入火力连发键").Disabled(a.busy).Clicked() {
				var target uint8 = 240
				switch a.fireTarget {
				case "鼠标左键":
					target = 240
				case "鼠标右键":
					target = 241
				case "鼠标中键":
					target = 242
				default:
					if kc, ok := hid.HIDKeyCode(a.fireTarget); ok {
						target = kc
					}
				}
				interval := uint8(math.Round(snapStep(a.fireInterval, 1, 255, 1)))
				cnt := uint8(math.Round(snapStep(a.fireCount, 0, 255, 1)))
				b := hid.FireKey(target, interval, cnt)
				a.applyKeyAction(slot, b.Code, fmt.Sprintf("火力连发 (%s %dms)", a.fireTarget, interval))
			}
		})
	})
}

func (a *DriverApp) viewKeyBindMedia(c *ui.Context, p StudioPalette, slot hid.KeySlot, curCode [4]byte) {
	actions := hid.MediaActions()
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, "完整收录 Windows 版多媒体播放、音量、浏览器与系统快捷指令，点击立即写入：").
			FontSize(12).TextColor(p.Theme.TextMuted)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, act := range actions {
				act := act
				active := curCode == act.Code
				if active {
					ui.PrimaryButton(c.Key("med-"+act.Name), act.Name).
						Padding(7, 13).Disabled(a.busy)
				} else if ui.Button(c.Key("med-"+act.Name), act.Name).
					Padding(7, 13).Disabled(a.busy).Clicked() {
					a.applyKeyAction(slot, act.Code, act.Name)
				}
			}
		})
	})
}

func (a *DriverApp) viewKeyBindMacro(c *ui.Context, p StudioPalette, slot hid.KeySlot) {
	ui.Column(c).Gap(14).Children(func() {
		ui.Text(c, "将此物理按键绑定至鼠标板载宏槽位（可在左侧「宏工作室」编排并写入对应槽位）：").
			FontSize(12).TextColor(p.Theme.TextMuted)

		ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "目标板载槽位").FontSize(13).Bold().Width(100)
			ui.Select(c.Key("bind-macro-slot"), &a.bindMacroSlot, macroSlotChoices).
				Label("目标板载宏槽位").Width(160)
			if ui.Button(c.Key("goto-macro-studio"), "前往宏工作室编辑 →").Clicked() {
				a.macroSlotLabel = a.bindMacroSlot
				a.activePage = pageMacro
			}
		})

		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "触发播放模式").FontSize(13).Bold()
			ui.Segmented(c.Key("bind-macro-mode"), &a.bindMacroMode, macroModeChoices...).
				FillEqually().Label("宏触发模式")
		})

		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.PrimaryButton(c.Key("apply-macro-bind"), "写入宏绑定").Disabled(a.busy).Clicked() {
				slotIdx := parseMacroSlotLabel(a.bindMacroSlot)
				mode := hid.MacroModeRepeatTimes
				switch a.bindMacroMode {
				case 1:
					mode = hid.MacroModeOnOff
				case 2:
					mode = hid.MacroModeTouchRepeat
				}
				code := hid.EncodeKeyAction(hid.KeyBinding{
					Kind:      hid.KeyKindMacro,
					MacroMode: mode,
					MacroSlot: slotIdx,
				})
				a.applyKeyAction(slot, code, fmt.Sprintf("宏槽位 %d (%s)", slotIdx+1, macroModeChoices[a.bindMacroMode]))
			}
		})
	})
}

func (a *DriverApp) viewKeyBindDisable(c *ui.Context, p StudioPalette, slot hid.KeySlot, curCode [4]byte) {
	disabled := curCode == [4]byte{}
	ui.Column(c).Gap(12).Padding(14).Radius(10).
		Background(p.Crimson.Alpha(0.08)).Border(1, p.Crimson.Alpha(0.24)).Children(func() {
		ui.Text(c, "禁用此物理按键").FontSize(14).Bold().TextColor(p.Crimson)
		ui.Text(c, "禁用后按下该按键将不会向系统上报任何 HID 事件，可用于防止游戏时误触侧键或 DPI 键。").
			FontSize(12).TextColor(p.Theme.TextMuted)
		ui.Row(c).Justify(ui.End).Children(func() {
			if disabled {
				statusPill(c, "该按键当前已禁用", p.Crimson)
			} else if ui.Button(c.Key("apply-disable-key"), "确认禁用此按键").
				TextColor(p.Crimson).Border(1, p.Crimson.Alpha(0.5)).
				Disabled(a.busy).Clicked() {
				a.applyKeyAction(slot, [4]byte{}, "禁用按键")
			}
		})
	})
}

func parseMacroSlotLabel(lbl string) uint8 {
	for i, s := range macroSlotChoices {
		if s == lbl {
			return uint8(i)
		}
	}
	return 0
}
