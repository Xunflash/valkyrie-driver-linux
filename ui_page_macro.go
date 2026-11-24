package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

var (
	macroAddKinds   = []string{"键盘单击", "鼠标单击", "鼠标移动", "纯延迟"}
	macroMouseBtns  = []string{"左键", "右键", "中键", "前进", "后退"}
	macroMouseCodes = map[string]uint8{
		"左键": 240, "右键": 241, "中键": 242, "前进": 244, "后退": 243,
	}
)

func (a *DriverApp) selectLocalMacro(m SavedMacro) {
	a.selectedMacroID = m.ID
	a.macroNameDraft = m.Name
	a.macroSlotLabel = fmt.Sprintf("宏槽位 %d", m.Slot+1)
	rep := m.Macro.RepeatCount
	if rep == 0 {
		rep = 1
	}
	a.macroRepeat = float64(rep)
	a.macroEvents = append([]hid.MacroEvent(nil), m.Macro.Events...)
}

func (a *DriverApp) newBlankMacro() {
	a.selectedMacroID = ""
	a.macroNameDraft = fmt.Sprintf("新宏 %d", len(a.savedMacros)+1)
	a.macroRepeat = 1
	a.macroEvents = nil
}

func (a *DriverApp) buildCurrentMacro() hid.Macro {
	slot := parseMacroSlotLabel(a.macroSlotLabel)
	rep := uint16(math.Round(snapStep(a.macroRepeat, 1, 65535, 1)))
	return hid.Macro{
		Name:        a.macroNameDraft,
		Slot:        slot,
		RepeatCount: rep,
		Events:      append([]hid.MacroEvent(nil), a.macroEvents...),
	}
}

func (a *DriverApp) macroUsedBytes() (int, error) {
	m := a.buildCurrentMacro()
	buf, err := hid.EncodeMacroBuffer(m)
	if err != nil {
		return 256, err
	}
	used := 2
	for i := len(buf) - 1; i >= 2; i-- {
		if buf[i] != 0 {
			used = i + 1
			break
		}
	}
	return used, nil
}

func (a *DriverApp) viewMacroPage(c *ui.Context, p StudioPalette) {
	pageTitle(c, p, "宏工作室 (Macro Studio)",
		"可视化编排键盘/鼠标按键与位移序列，直接读写鼠标板载 16 组宏槽位 (0x83 / 0x03)",
		func() {
			if ui.Button(c.Key("macro-new-btn"), "+ 新建空白宏").Clicked() {
				a.newBlankMacro()
			}
			if ui.Button(c.Key("macro-save-local"), "保存到本地库").Disabled(a.store == nil).Clicked() {
				a.saveCurrentMacroLocal()
			}
			if ui.PrimaryButton(c.Key("macro-write-mouse"), "写入鼠标板载槽位").Padding(7, 14).Disabled(a.busy).Clicked() {
				a.writeCurrentMacroToMouse()
			}
		})

	ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
		// Left Column: Local Macro Library & Onboard Hardware Read/Write
		ui.Column(c).Width(254).Shrink(0).Gap(14).Children(func() {
			studioCard(c, p, "板载宏槽位同步", "读写鼠标硬件内置的 16 个宏存储区", func() {
				ui.Column(c).Gap(10).Children(func() {
					ui.Select(c.Key("macro-slot-sel"), &a.macroSlotLabel, macroSlotChoices).
						Label("板载宏槽位").FillWidth()
					ui.Row(c).Gap(8).Children(func() {
						if ui.Button(c.Key("macro-read-slot"), "从鼠标读取").Grow(1).Disabled(a.busy).Clicked() {
							slot := parseMacroSlotLabel(a.macroSlotLabel)
							a.runOp(func() func() {
								m, err := a.backend.ReadMacro(slot)
								return func() {
									if err != nil {
										a.setToast("读取板载宏失败: "+err.Error(), true)
										return
									}
									a.selectedMacroID = ""
									a.macroNameDraft = fmt.Sprintf("板载槽位 %d", slot+1)
									rep := m.RepeatCount
									if rep == 0 {
										rep = 1
									}
									a.macroRepeat = float64(rep)
									a.macroEvents = m.Events
									a.setToast(fmt.Sprintf("已从槽位 %d 读取 %d 条动作", slot+1, len(m.Events)), false)
								}
							})
						}
						if ui.Button(c.Key("macro-clear-events"), "清空动作").Disabled(len(a.macroEvents) == 0).Clicked() {
							a.macroEvents = nil
						}
					})
				})
			})

			studioCard(c, p, "本地宏库", fmt.Sprintf("共保存 %d 个宏脚本", len(a.savedMacros)), func() {
				if len(a.savedMacros) == 0 {
					ui.Text(c, "暂无已保存的本地宏。在右侧添加动作后点击「保存到本地库」。").
						FontSize(12).TextColor(p.Theme.TextMuted)
				} else {
					ui.Column(c).Gap(6).Children(func() {
						for _, item := range a.savedMacros {
							item := item
							sel := item.ID == a.selectedMacroID
							row := ui.Row(c.Key("mlib-"+item.ID)).Padding(8, 10).Radius(9).
								Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(6)
							if sel {
								row.Background(p.AccentGlow).Border(1, p.Theme.Accent.Alpha(0.4))
							} else {
								row.Background(p.CardElevated).Border(1, p.CardBorder)
							}
							row.Children(func() {
								pick := ui.ButtonBase(c.Key("mpick-"+item.ID)).Grow(1).Label(item.Name)
								if pick.Clicked() {
									a.selectLocalMacro(item)
								}
								pick.Children(func() {
									ui.Column(c).Gap(2).FillWidth().Children(func() {
										ui.Text(c, item.Name).FontSize(12).Bold().SingleLine()
										ui.Textf(c, "槽位 %d · %d 步 · 重复 %d 次",
											item.Slot+1, len(item.Macro.Events), item.Macro.RepeatCount).
											FontSize(10).TextColor(p.SubtleText).SingleLine()
									})
								})
								if ui.Button(c.Key("mdel-"+item.ID), "删").Padding(3, 7).TextColor(p.Crimson).Clicked() {
									if a.store != nil {
										_ = a.store.DeleteMacro(item.ID)
										if a.selectedMacroID == item.ID {
											a.selectedMacroID = ""
										}
										a.loadLocalData()
										a.setToast("已删除宏「"+item.Name+"」", false)
									}
								}
							})
						}
					})
				}
			})
		})

		// Right Column: Macro Properties, Step Composer & Timeline
		ui.Column(c).Grow(1).Gap(14).Children(func() {
			usedBytes, encodeErr := a.macroUsedBytes()
			studioCard(c, p, "宏脚本属性与容量", "", func() {
				ui.Row(c).Gap(12).AlignItems(ui.End).Wrap().Children(func() {
					ui.Column(c).Gap(5).Grow(1).MinWidth(180).Children(func() {
						ui.Text(c, "宏名称").FontSize(12).Bold()
						ui.TextInput(c.Key("macro-name-in"), &a.macroNameDraft).Placeholder("例如：一键连招 / 办公粘贴").Label("宏名称")
					})
					ui.Column(c).Gap(5).Width(120).Children(func() {
						ui.Text(c, "默认循环次数").FontSize(12).Bold()
						ui.NumberInput(c.Key("macro-repeat-in"), &a.macroRepeat, 1, 65535, 1).Label("循环次数")
					})
					ui.Column(c).Gap(5).Width(150).Children(func() {
						ui.Row(c).Justify(ui.SpaceBetween).Children(func() {
							ui.Text(c, "板载容量").FontSize(11).TextColor(p.Theme.TextMuted)
							ui.Textf(c, "%d / 256 B", usedBytes).Font("monospace").FontSize(11).Bold()
						})
						ui.Meter(c, float64(usedBytes), 0, 256, &ui.MeterLevels{
							Warning:       200,
							Critical:      240,
							NormalColor:   p.Cyan,
							WarningColor:  p.Amber,
							CriticalColor: p.Crimson,
						}).Height(6).Label("板载宏容量")
					})
				})
				if encodeErr != nil {
					ui.Text(c, "编码提示："+encodeErr.Error()).FontSize(11).TextColor(p.Crimson)
				}
			})

			// Step Composer Bar
			studioCard(c, p, "追加宏动作", "快速向时间轴末尾追加键盘单击、鼠标点击、相对位移或延迟", func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Segmented(c.Key("macro-add-kind"), &a.macroAddKind, macroAddKinds...).
						FillEqually().Label("追加动作类型")

					ui.Row(c).Gap(10).AlignItems(ui.End).Wrap().Children(func() {
						switch a.macroAddKind {
						case 0: // Keyboard click (down + delay + up + delay)
							ui.Column(c).Gap(4).Width(140).Children(func() {
								ui.Text(c, "键盘按键").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.Select(c.Key("macro-add-key"), &a.macroAddKey, comboKeyChoices).Label("宏键盘按键")
							})
							ui.Column(c).Gap(4).Width(120).Children(func() {
								ui.Text(c, "按住/间隔 (ms)").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-add-delay"), &a.macroAddDelay, 1, 5000, 10).Label("延迟毫秒")
							})
							if ui.PrimaryButton(c.Key("macro-append-btn"), "+ 追加键盘单击").Clicked() {
								if kc, ok := hid.HIDKeyCode(a.macroAddKey); ok {
									d := uint16(math.Round(snapStep(a.macroAddDelay, 1, 5000, 1)))
									a.macroEvents = append(a.macroEvents,
										hid.MacroEvent{Type: hid.MacroKeyboard, Action: hid.MacroActionDown, KeyCode: kc},
										hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d},
										hid.MacroEvent{Type: hid.MacroKeyboard, Action: hid.MacroActionUp, KeyCode: kc},
										hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d},
									)
								}
							}
						case 1: // Mouse click
							ui.Column(c).Gap(4).Width(120).Children(func() {
								ui.Text(c, "鼠标按键").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.Select(c.Key("macro-add-mbtn"), &a.macroAddMouse, macroMouseBtns).Label("宏鼠标按键")
							})
							ui.Column(c).Gap(4).Width(120).Children(func() {
								ui.Text(c, "按住/间隔 (ms)").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-add-mdelay"), &a.macroAddDelay, 1, 5000, 10).Label("延迟毫秒")
							})
							if ui.PrimaryButton(c.Key("macro-append-mbtn"), "+ 追加鼠标单击").Clicked() {
								mc := macroMouseCodes[a.macroAddMouse]
								if mc == 0 {
									mc = 240
								}
								d := uint16(math.Round(snapStep(a.macroAddDelay, 1, 5000, 1)))
								a.macroEvents = append(a.macroEvents,
									hid.MacroEvent{Type: hid.MacroMouseButton, Action: hid.MacroActionDown, KeyCode: mc},
									hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d},
									hid.MacroEvent{Type: hid.MacroMouseButton, Action: hid.MacroActionUp, KeyCode: mc},
									hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d},
								)
							}
						case 2: // Mouse relative move
							ui.Column(c).Gap(4).Width(96).Children(func() {
								ui.Text(c, "X 位移").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-dx"), &a.macroAddDX, -127, 127, 1).Label("DX")
							})
							ui.Column(c).Gap(4).Width(96).Children(func() {
								ui.Text(c, "Y 位移").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-dy"), &a.macroAddDY, -127, 127, 1).Label("DY")
							})
							ui.Column(c).Gap(4).Width(110).Children(func() {
								ui.Text(c, "后置延迟 (ms)").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-mv-delay"), &a.macroAddDelay, 1, 5000, 10).Label("延迟毫秒")
							})
							if ui.PrimaryButton(c.Key("macro-append-mv"), "+ 追加位移").Clicked() {
								d := uint16(math.Round(snapStep(a.macroAddDelay, 1, 5000, 1)))
								a.macroEvents = append(a.macroEvents,
									hid.MacroEvent{
										Type: hid.MacroMouseMove,
										DX:   int8(math.Round(snapStep(a.macroAddDX, -127, 127, 1))),
										DY:   int8(math.Round(snapStep(a.macroAddDY, -127, 127, 1))),
									},
									hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d},
								)
							}
						case 3: // Pure delay
							ui.Column(c).Gap(4).Width(130).Children(func() {
								ui.Text(c, "延迟时长 (ms)").FontSize(11).TextColor(p.Theme.TextMuted)
								ui.NumberInput(c.Key("macro-pure-delay"), &a.macroAddDelay, 1, 65535, 10).Label("纯延迟毫秒")
							})
							if ui.PrimaryButton(c.Key("macro-append-delay"), "+ 追加延迟").Clicked() {
								d := uint16(math.Round(snapStep(a.macroAddDelay, 1, 65535, 1)))
								a.macroEvents = append(a.macroEvents, hid.MacroEvent{Type: hid.MacroDelay, DelayMs: d})
							}
						}
					})
				})
			})

			// Timeline List
			studioCard(c, p, fmt.Sprintf("动作时间轴（共 %d 步）", len(a.macroEvents)), "", func() {
				if len(a.macroEvents) == 0 {
					ui.Text(c, "当前时间轴为空，请在上方追加动作或从鼠标板载槽位读取已有宏。").
						FontSize(12).TextColor(p.Theme.TextMuted)
				} else {
					ui.Column(c).Gap(6).Children(func() {
						for i, ev := range a.macroEvents {
							i := i
							ev := ev
							tag, desc, col := describeMacroEvent(ev, p)
							ui.Row(c.Key(fmt.Sprintf("mev-%d", i))).Padding(7, 10).Radius(8).
								Background(p.CardElevated).Border(1, p.CardBorder).
								Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(10).Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Children(func() {
									ui.Textf(c, "%02d", i+1).Font("monospace").FontSize(11).TextColor(p.SubtleText)
									statusPill(c, tag, col)
									ui.Text(c, desc).FontSize(12).Bold()
								})
								ui.Row(c).Gap(4).Children(func() {
									if ui.Button(c.Key(fmt.Sprintf("mev-up-%d", i)), "↑").Padding(2, 7).Disabled(i == 0).Clicked() {
										a.macroEvents[i-1], a.macroEvents[i] = a.macroEvents[i], a.macroEvents[i-1]
									}
									if ui.Button(c.Key(fmt.Sprintf("mev-dn-%d", i)), "↓").Padding(2, 7).Disabled(i == len(a.macroEvents)-1).Clicked() {
										a.macroEvents[i+1], a.macroEvents[i] = a.macroEvents[i], a.macroEvents[i+1]
									}
									if ui.Button(c.Key(fmt.Sprintf("mev-rm-%d", i)), "×").Padding(2, 7).TextColor(p.Crimson).Clicked() {
										a.macroEvents = append(a.macroEvents[:i], a.macroEvents[i+1:]...)
									}
								})
							})
						}
					})
				}
			})
		})
	})
}

func describeMacroEvent(ev hid.MacroEvent, p StudioPalette) (string, string, ui.Color) {
	switch ev.Type {
	case hid.MacroKeyboard:
		name := fmt.Sprintf("HID %d", ev.KeyCode)
		if n, ok := hid.HIDKeyName(ev.KeyCode); ok {
			name = n
		}
		if ev.Action == hid.MacroActionDown {
			return "键盘按下", name, p.Cyan
		}
		return "键盘抬起", name, p.SubtleText
	case hid.MacroMouseButton:
		name := mouseBtnName(ev.KeyCode)
		if ev.Action == hid.MacroActionDown {
			return "鼠标按下", name, p.Violet
		}
		return "鼠标抬起", name, p.SubtleText
	case hid.MacroMouseMove:
		return "鼠标移动", fmt.Sprintf("ΔX=%+d, ΔY=%+d", ev.DX, ev.DY), p.Amber
	case hid.MacroDelay:
		return "延迟等待", fmt.Sprintf("%d ms", ev.DelayMs), p.Emerald
	default:
		return ev.Type, "", p.Theme.TextMuted
	}
}

func mouseBtnName(code uint8) string {
	switch code {
	case 240:
		return "左键"
	case 241:
		return "右键"
	case 242:
		return "中键"
	case 243:
		return "后退"
	case 244:
		return "前进"
	default:
		return fmt.Sprintf("键 %d", code)
	}
}

func (a *DriverApp) saveCurrentMacroLocal() {
	if a.store == nil {
		return
	}
	m := a.buildCurrentMacro()
	saved, err := a.store.SaveMacro(SavedMacro{
		ID:    a.selectedMacroID,
		Name:  a.macroNameDraft,
		Slot:  m.Slot,
		Macro: m,
	})
	if err != nil {
		a.setToast("保存本地宏失败: "+err.Error(), true)
		return
	}
	a.selectedMacroID = saved.ID
	a.loadLocalData()
	a.setToast("已保存宏「"+saved.Name+"」到本地库", false)
}

func (a *DriverApp) writeCurrentMacroToMouse() {
	m := a.buildCurrentMacro()
	if _, err := hid.EncodeMacroBuffer(m); err != nil {
		a.setToast("宏格式校验失败: "+err.Error(), true)
		return
	}
	slot := m.Slot
	a.runOp(func() func() {
		err := a.backend.WriteMacro(slot, m)
		return func() {
			if err != nil {
				a.setToast("写入板载宏失败: "+err.Error(), true)
				return
			}
			a.setToast(fmt.Sprintf("宏「%s」已写入鼠标槽位 %d", m.Name, slot+1), false)
		}
	})
}
