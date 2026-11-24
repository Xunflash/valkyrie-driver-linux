package main

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/vkm5.png
var vkm5PNG []byte

var (
	vkm5BitmapOnce sync.Once
	vkm5Bitmap     *ui.Bitmap
)

func getVKM5Bitmap() *ui.Bitmap {
	vkm5BitmapOnce.Do(func() {
		if len(vkm5PNG) > 0 {
			if bmp, err := ui.DecodeBitmap(vkm5PNG); err == nil {
				vkm5Bitmap = bmp
			}
		}
	})
	return vkm5Bitmap
}

// Crisp 24x24 Lucide-style vector icons rendered natively on GPU by MyGo.
var (
	iconBrand = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 5l8 15L20 5"/><path d="M9.5 5L12 10l2.5-5"/></svg>`))
	iconMouse = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="3" width="12" height="18" rx="6"/><path d="M12 7v4"/></svg>`))
	iconDPI   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3"/><path d="M12 3v3M12 18v3M3 12h3M18 12h3"/></svg>`))
	iconPerf  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2L4 14h7l-1 8 10-12h-7l0-8z"/></svg>`))
	iconTune  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="2.5"/><circle cx="15" cy="17" r="2.5"/></svg>`))
	iconMacro = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="3"/><path d="M8 10l3 2-3 2M13 14h4"/></svg>`))
	iconLayer = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 4.5-9 4.5-9-4.5L12 3z"/><path d="M3 12l9 4.5 9-4.5"/><path d="M3 16.5L12 21l9-4.5"/></svg>`))
	iconGear  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.08a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.08a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`))
	iconSync  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/></svg>`))
)

// StudioPalette extends MyGo's Theme with layered obsidian/porcelain hardware studio tokens.
type StudioPalette struct {
	Theme          *ui.Theme
	Dark           bool
	Canvas         ui.Color
	Sidebar        ui.Color
	Card           ui.Color
	CardElevated   ui.Color
	CardBorder     ui.Color
	CardHighlight  ui.Color
	CardShadow     ui.Color
	SubtleText     ui.Color
	AccentGlow     ui.Color
	Cyan           ui.Color
	Emerald        ui.Color
	Violet         ui.Color
	Crimson        ui.Color
	Amber          ui.Color
}

func resolvePalette(c *ui.Context, mode string) StudioPalette {
	dark := c.Theme().Dark
	switch mode {
	case "dark":
		dark = true
	case "light":
		dark = false
	}

	if dark {
		t := ui.DarkTheme()
		t.Background = ui.Hex("#080B11")
		t.Surface = ui.Hex("#111723")
		t.SurfaceHover = ui.Hex("#1A2336")
		t.SurfacePressed = ui.Hex("#232F47")
		t.Border = ui.Hex("#202B40")
		t.Text = ui.Hex("#F1F5F9")
		t.TextMuted = ui.Hex("#8896AE")
		t.Accent = ui.Hex("#38BDF8")
		t.AccentHover = ui.Hex("#7DD3FC")
		t.AccentPressed = ui.Hex("#0EA5E9")
		t.AccentText = ui.Hex("#06131F")
		t.Success = ui.Hex("#34D399")
		t.Warning = ui.Hex("#FBBF24")
		t.Danger = ui.Hex("#FB7185")
		t.Focus = ui.RGBA(56, 189, 248, 0.55)
		t.Selection = ui.RGBA(56, 189, 248, 0.28)
		t.Radius = 9
		c.SetTheme(t)
		return StudioPalette{
			Theme:         t,
			Dark:          true,
			Canvas:        ui.Hex("#080B11"),
			Sidebar:       ui.Hex("#0C1019"),
			Card:          ui.Hex("#111723"),
			CardElevated:  ui.Hex("#161E2E"),
			CardBorder:    ui.RGBA(148, 163, 184, 0.14),
			CardHighlight: ui.RGBA(255, 255, 255, 0.06),
			CardShadow:    ui.RGBA(0, 0, 0, 0.36),
			SubtleText:    ui.Hex("#64748B"),
			AccentGlow:    ui.RGBA(56, 189, 248, 0.16),
			Cyan:          ui.Hex("#38BDF8"),
			Emerald:       ui.Hex("#34D399"),
			Violet:        ui.Hex("#A78BFA"),
			Crimson:       ui.Hex("#FB7185"),
			Amber:         ui.Hex("#FBBF24"),
		}
	}

	t := ui.LightTheme()
	t.Background = ui.Hex("#F2F5FA")
	t.Surface = ui.Hex("#FFFFFF")
	t.SurfaceHover = ui.Hex("#F1F5F9")
	t.SurfacePressed = ui.Hex("#E2E8F0")
	t.Border = ui.Hex("#DCE4F0")
	t.Text = ui.Hex("#0F172A")
	t.TextMuted = ui.Hex("#64748B")
	t.Accent = ui.Hex("#0284C7")
	t.AccentHover = ui.Hex("#0369A1")
	t.AccentPressed = ui.Hex("#075985")
	t.AccentText = ui.Hex("#FFFFFF")
	t.Success = ui.Hex("#10B981")
	t.Warning = ui.Hex("#D97706")
	t.Danger = ui.Hex("#E11D48")
	t.Focus = ui.RGBA(2, 132, 199, 0.45)
	t.Selection = ui.RGBA(2, 132, 199, 0.20)
	t.Radius = 9
	c.SetTheme(t)
	return StudioPalette{
		Theme:         t,
		Dark:          false,
		Canvas:        ui.Hex("#F2F5FA"),
		Sidebar:       ui.Hex("#FFFFFF"),
		Card:          ui.Hex("#FFFFFF"),
		CardElevated:  ui.Hex("#F8FAFC"),
		CardBorder:    ui.RGBA(15, 23, 42, 0.09),
		CardHighlight: ui.RGBA(255, 255, 255, 0.90),
		CardShadow:    ui.RGBA(15, 23, 42, 0.06),
		SubtleText:    ui.Hex("#94A3B8"),
		AccentGlow:    ui.RGBA(2, 132, 199, 0.12),
		Cyan:          ui.Hex("#0284C7"),
		Emerald:       ui.Hex("#10B981"),
		Violet:        ui.Hex("#7C3AED"),
		Crimson:       ui.Hex("#E11D48"),
		Amber:         ui.Hex("#D97706"),
	}
}

// studioCard renders a layered hardware-studio card with hairline border, specular top sheen, and elevation shadow.
func studioCard(c *ui.Context, p StudioPalette, title, subtitle string, body func()) ui.Element {
	el := ui.Column(c).Padding(18).Gap(14).Radius(14).
		Background(p.Card).
		Border(1, p.CardBorder).
		InsetShadow(0, 1, 0, 0, p.CardHighlight).
		Shadow(0, 8, 24, 0, p.CardShadow)
	el.Children(func() {
		if title != "" {
			ui.Column(c).Gap(3).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(3, 14).Radius(2).Background(p.Theme.Accent)
					ui.Text(c, title).FontSize(14).Bold()
				})
				if subtitle != "" {
					ui.Text(c, subtitle).FontSize(12).TextColor(p.Theme.TextMuted)
				}
			})
		}
		body()
	})
	return el
}

// pageTitle renders the clean page title header with optional right-side action slot.
func pageTitle(c *ui.Context, p StudioPalette, title, subtitle string, right func()) {
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(12).Padding(0, 0, 4, 0).Children(func() {
		ui.Column(c).Gap(3).Grow(1).Children(func() {
			ui.Text(c, title).FontSize(22).Bold()
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(12).TextColor(p.Theme.TextMuted)
			}
		})
		if right != nil {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(right)
		}
	})
}

// statusPill draws a refined tinted capsule badge that hugs its label tightly.
func statusPill(c *ui.Context, text string, fg ui.Color) {
	ui.Row(c).AlignSelf(ui.Start).Padding(3, 10).Radius(999).AlignItems(ui.Center).
		Background(fg.Alpha(0.15)).Border(1, fg.Alpha(0.28)).Children(func() {
		ui.Text(c, text).FontSize(11).Bold().TextColor(fg).SingleLine()
	})
}

// studioInfoRow renders a key-value diagnostic row.
func studioInfoRow(c *ui.Context, p StudioPalette, label, val string) {
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Padding(4, 0).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(p.Theme.TextMuted)
		ui.Text(c, val).FontSize(13).Font("monospace").Selectable()
	})
}

// formatSleepDuration formats seconds into a friendly Chinese duration hint.
func formatSleepDuration(sec uint16) string {
	if sec == 0 {
		return "永不休眠"
	}
	if sec < 60 {
		return fmt.Sprintf("%d 秒", sec)
	}
	mins := sec / 60
	rem := sec % 60
	if rem == 0 {
		return fmt.Sprintf("%d 分钟 (%d 秒)", mins, sec)
	}
	return fmt.Sprintf("%d 分 %d 秒", mins, rem)
}
