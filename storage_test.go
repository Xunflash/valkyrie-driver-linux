package main

import (
	"bytes"
	"testing"

	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

func TestLocalStoreProfilesMacrosPrefs(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.LoadProfiles()
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty profiles: %v %v", empty, err)
	}
	prefs, err := store.LoadPreferences()
	if err != nil || prefs.ThemeMode != "system" {
		t.Fatalf("default prefs %+v err=%v", prefs, err)
	}
	if err := store.SavePreferences(AppPreferences{ThemeMode: "dark"}); err != nil {
		t.Fatal(err)
	}
	prefs, err = store.LoadPreferences()
	if err != nil || prefs.ThemeMode != "dark" {
		t.Fatalf("saved prefs %+v err=%v", prefs, err)
	}
	if err := store.SavePreferences(AppPreferences{ThemeMode: "blue"}); err == nil {
		t.Fatal("expected unknown theme to fail")
	}

	prof, err := store.SaveProfile(SavedProfile{
		Name: "日常",
		DPI: hid.DPIConfig{
			Current: 1,
			Count:   2,
			Stages:  []hid.DPIStage{{X: 800, Y: 800, R: 255}, {X: 1600, Y: 1600, G: 255}},
		},
		Params: hid.MouseParams{ReportRate: 1000, MotionSync: true, Debounce: 3},
		Keys:   hid.DefaultMatrix(),
	})
	if err != nil || prof.ID == "" || prof.UpdatedAt.IsZero() {
		t.Fatalf("save profile %+v err=%v", prof, err)
	}
	prof.Params.ReportRate = 500
	saved, err := store.SaveProfile(prof)
	if err != nil || saved.Params.ReportRate != 500 {
		t.Fatalf("update %+v err=%v", saved, err)
	}
	renamed, err := store.RenameProfile(prof.ID, "办公")
	if err != nil || renamed.Name != "办公" {
		t.Fatalf("rename %+v err=%v", renamed, err)
	}
	blob, err := store.ExportProfileJSON(prof.ID)
	if err != nil || !bytes.Contains(blob, []byte(`"办公"`)) {
		t.Fatalf("export %s err=%v", blob, err)
	}
	imported, err := store.ImportProfileJSON(blob)
	if err != nil || imported.ID != prof.ID || imported.DPI.Stages[1].X != 1600 {
		t.Fatalf("import %+v err=%v", imported, err)
	}
	items, err := store.LoadProfiles()
	if err != nil || len(items) != 1 {
		t.Fatalf("profiles %+v err=%v", items, err)
	}

	macro, err := store.SaveMacro(SavedMacro{
		Name: "开火",
		Slot: 2,
		Macro: hid.Macro{
			RepeatCount: 1,
			Events: []hid.MacroEvent{{
				Type: hid.MacroKeyboard, Action: hid.MacroActionDown, KeyCode: 4, DelayMs: 10,
			}},
		},
	})
	if err != nil || macro.Macro.Slot != 2 || macro.Macro.Name != "开火" {
		t.Fatalf("save macro %+v err=%v", macro, err)
	}
	if _, err := store.SaveMacro(SavedMacro{Name: "bad", Slot: 16}); err == nil {
		t.Fatal("expected slot 16 to fail")
	}
	macros, err := store.LoadMacros()
	if err != nil || len(macros) != 1 || macros[0].Macro.Events[0].KeyCode != 4 {
		t.Fatalf("macros %+v err=%v", macros, err)
	}
	if err := store.DeleteMacro(macro.ID); err != nil {
		t.Fatal(err)
	}
	if macros, err = store.LoadMacros(); err != nil || len(macros) != 0 {
		t.Fatalf("macros after delete %+v err=%v", macros, err)
	}
	if err := store.DeleteProfile(prof.ID); err != nil {
		t.Fatal(err)
	}
	if items, err = store.LoadProfiles(); err != nil || len(items) != 0 {
		t.Fatalf("profiles after delete %+v err=%v", items, err)
	}
	if err := store.DeleteProfile(prof.ID); err == nil {
		t.Fatal("expected missing profile error")
	}
}
