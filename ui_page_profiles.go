package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *DriverApp) viewProfilesPage(c *ui.Context, p StudioPalette) {
	pageTitle(c, p, "本地配置方案",
		"将鼠标完整快照（DPI / 性能 / 传感器 / 8 键映射）保存为命名方案，支持一键切换与 JSON 导入导出",
		func() {
			if ui.Button(c.Key("prof-open-import"), "导入 JSON 方案").Disabled(a.store == nil).Clicked() {
				a.importJSONText = ""
				a.importModalOpen = true
			}
		})

	// Save current mouse snapshot card
	studioCard(c, p, "保存当前鼠标状态为本地配置方案",
		"完整记录当前 DPI 档位、回报率、静默高度、传感器算法、防抖/休眠参数以及全部按键映射",
		func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c.Key("new-prof-name"), &a.newProfileName).
					Placeholder("输入方案名称，例如：FPS 竞技 / 静音办公 / MOBA").
					Label("新配置方案名称").Grow(1)
				if ui.PrimaryButton(c.Key("save-prof-btn"), "保存为新方案").
					Padding(7, 16).Disabled(a.store == nil || a.state == nil).Clicked() {
					name := strings.TrimSpace(a.newProfileName)
					if name == "" {
						name = fmt.Sprintf("配置方案 %d", len(a.savedProfiles)+1)
					}
					saved, err := a.store.SaveProfile(SavedProfile{
						Name:   name,
						DPI:    a.state.DPI.Clone(),
						Params: a.state.Params,
						Keys:   a.state.Keys,
					})
					if err != nil {
						a.setToast("保存方案失败: "+err.Error(), true)
					} else {
						a.newProfileName = ""
						a.loadLocalData()
						a.setToast("已保存本地配置「"+saved.Name+"」", false)
					}
				}
			})
		})

	// Saved profiles grid
	studioCard(c, p, fmt.Sprintf("已保存的配置方案（%d）", len(a.savedProfiles)), "", func() {
		if len(a.savedProfiles) == 0 {
			ui.Text(c, "暂无本地配置方案。可在上方保存当前鼠标配置，或导入他人分享的 JSON 方案。").
				FontSize(12).TextColor(p.Theme.TextMuted)
			return
		}
		ui.Column(c).Gap(10).Children(func() {
			for _, prof := range a.savedProfiles {
				prof := prof
				activeDPI := uint16(0)
				if prof.DPI.Current >= 0 && prof.DPI.Current < len(prof.DPI.Stages) {
					activeDPI = prof.DPI.Stages[prof.DPI.Current].X
				}
				ui.Row(c.Key("pcard-"+prof.ID)).Padding(14, 16).Radius(12).
					Background(p.CardElevated).Border(1, p.CardBorder).
					Justify(ui.SpaceBetween).AlignItems(ui.Center).Wrap().Gap(12).Children(func() {
					ui.Column(c).Gap(6).Grow(1).Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
							ui.Text(c, prof.Name).FontSize(15).Bold()
							statusPill(c, fmt.Sprintf("%d DPI", activeDPI), p.Cyan)
							statusPill(c, fmt.Sprintf("%d Hz", prof.Params.ReportRate), p.Violet)
							if prof.Params.MotionSync {
								statusPill(c, "MotionSync", p.Emerald)
							}
						})
						ui.Textf(c, "档位数 %d · 防抖 %dms · 更新于 %s",
							len(prof.DPI.Stages), prof.Params.Debounce,
							prof.UpdatedAt.Local().Format("2006-01-02 15:04")).
							FontSize(11).TextColor(p.SubtleText)
					})

					ui.Row(c).Gap(7).AlignItems(ui.Center).Wrap().Children(func() {
						if ui.PrimaryButton(c.Key("papply-"+prof.ID), "应用到鼠标").
							Padding(5, 12).Disabled(a.busy).Clicked() {
							target := prof
							a.runOp(func() func() {
								st, err := a.backend.ApplyProfileSnapshot(target)
								return func() {
									if err != nil {
										a.setToast("应用方案失败: "+err.Error(), true)
										return
									}
									a.selectedStage = st.DPI.Current
									a.syncFromState(st)
									a.setToast("已将方案「"+target.Name+"」完整写入鼠标", false)
								}
							})
						}
						if ui.Button(c.Key("prename-"+prof.ID), "重命名").Padding(5, 10).Clicked() {
							a.renameProfileID = prof.ID
							a.renameProfileDraft = prof.Name
							a.renameModalOpen = true
						}
						if ui.Button(c.Key("pexport-"+prof.ID), "导出").Padding(5, 10).Clicked() {
							if a.store != nil {
								data, err := a.store.ExportProfileJSON(prof.ID)
								if err != nil {
									a.setToast("导出失败: "+err.Error(), true)
								} else {
									a.exportTitle = prof.Name
									a.exportJSONText = string(data)
									a.exportModalOpen = true
								}
							}
						}
						if ui.Button(c.Key("pdel-"+prof.ID), "删除").Padding(5, 10).TextColor(p.Crimson).Clicked() {
							if a.store != nil {
								if err := a.store.DeleteProfile(prof.ID); err != nil {
									a.setToast("删除失败: "+err.Error(), true)
								} else {
									a.loadLocalData()
									a.setToast("已删除方案「"+prof.Name+"」", false)
								}
							}
						}
					})
				})
			}
		})
	})
}

func (a *DriverApp) viewProfileModals(c *ui.Context, p StudioPalette) {
	// Rename Profile Modal
	ui.Modal(c, &a.renameModalOpen, func() {
		ui.Column(c).Width(360).Gap(12).Children(func() {
			ui.Text(c, "重命名配置方案").FontSize(16).Bold()
			submitted := ui.TextInput(c.Key("rename-prof-in"), &a.renameProfileDraft).
				AutoFocus().Label("新方案名称").Submitted()
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c.Key("rename-cancel"), "取消").Clicked() {
					a.renameModalOpen = false
				}
				if ui.PrimaryButton(c.Key("rename-ok"), "保存名称").Clicked() || submitted {
					if a.store != nil {
						if _, err := a.store.RenameProfile(a.renameProfileID, strings.TrimSpace(a.renameProfileDraft)); err != nil {
							a.setToast("重命名失败: "+err.Error(), true)
						} else {
							a.loadLocalData()
							a.renameModalOpen = false
							a.setToast("配置方案已重命名", false)
						}
					}
				}
			})
		})
	})

	// Import JSON Modal
	ui.Modal(c, &a.importModalOpen, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			ui.Text(c, "导入配置方案 (JSON)").FontSize(16).Bold()
			ui.Text(c, "粘贴此前导出的配置方案 JSON 文本，导入后将加入本地配置方案列表：").
				FontSize(12).TextColor(p.Theme.TextMuted)
			ui.TextArea(c.Key("import-json-area"), &a.importJSONText).
				Height(200).Font("monospace").FontSize(11).Label("配置 JSON")
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c.Key("import-cancel"), "取消").Clicked() {
					a.importModalOpen = false
				}
				if ui.PrimaryButton(c.Key("import-confirm"), "确认导入").Clicked() {
					if a.store != nil {
						saved, err := a.store.ImportProfileJSON([]byte(a.importJSONText))
						if err != nil {
							a.setToast("导入失败: "+err.Error(), true)
						} else {
							a.loadLocalData()
							a.importModalOpen = false
							a.setToast("已导入方案「"+saved.Name+"」", false)
						}
					}
				}
			})
		})
	})

	// Export JSON Modal
	ui.Modal(c, &a.exportModalOpen, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			ui.Textf(c, "导出配置方案 · %s", a.exportTitle).FontSize(16).Bold()
			ui.TextArea(c.Key("export-json-area"), &a.exportJSONText).
				ReadOnly(true).Height(210).Font("monospace").FontSize(11).Label("导出 JSON")
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).Children(func() {
				if ui.Button(c.Key("export-save-file"), "保存到 ~/Downloads").Clicked() {
					if home, err := os.UserHomeDir(); err == nil {
						safe := strings.ReplaceAll(a.exportTitle, "/", "_")
						dst := filepath.Join(home, "Downloads", "valkyrie-"+safe+".json")
						if werr := os.WriteFile(dst, []byte(a.exportJSONText), 0o644); werr != nil {
							a.setToast("写入文件失败: "+werr.Error(), true)
						} else {
							a.setToast("已导出至 "+dst, false)
						}
					}
				}
				if ui.PrimaryButton(c.Key("export-close"), "关闭").Clicked() {
					a.exportModalOpen = false
				}
			})
		})
	})
}
