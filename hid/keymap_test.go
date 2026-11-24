package hid

import "testing"

func TestDefaultMatrixAndSlots(t *testing.T) {
	mx := DefaultMatrix()
	if mx[0] != [4]byte{1, 0, 240, 0} || mx[5] != [4]byte{20, 0, 0, 0} || mx[6] != [4]byte{10, 5, 0, 0} {
		t.Fatalf("default matrix header mismatch: %+v", mx[:7])
	}
	if mx[12] != [4]byte{1, 0, 249, 1} || mx[15] != [4]byte{1, 0, 245, 255} {
		t.Fatalf("default matrix wheel mismatch: %+v", mx[12:])
	}
	if len(ConfigurableSlots) != 8 {
		t.Fatalf("slots = %d, want 8", len(ConfigurableSlots))
	}
	if ConfigurableSlots[0].Order != 1 || ConfigurableSlots[7].Index != 5 || ConfigurableSlots[7].HotspotX != 172 {
		t.Fatalf("slot order/hotspot mismatch: %+v", ConfigurableSlots)
	}
	if ConfigurableSlots[5].Index != 14 || ConfigurableSlots[5].DefaultAction != [4]byte{1, 0, 245, 1} {
		t.Fatalf("wheel up slot: %+v", ConfigurableSlots[5])
	}
}

func TestModelSpecDefaults(t *testing.T) {
	m5 := LookupModel(2914, "")
	if len(m5.DefaultDPI) != 6 || m5.DefaultDPI[2] != 1200 || m5.DefaultCurrentDPI != 2 || m5.SupportsTrue8K {
		t.Fatalf("VK M5 defaults: %+v", m5)
	}
	if m5.BatteryHours[PerfOffice] != "270-300" || m5.BatteryHours[PerfRage] != "45-60" {
		t.Fatalf("VK M5 hours: %+v", m5.BatteryHours)
	}
	lite := LookupModel(3090, "")
	if lite.BatteryHours[PerfOffice] != "280-310" || lite.BatteryHours[PerfPerformance] != "175-200" {
		t.Fatalf("3311 hours: %+v", lite.BatteryHours)
	}
	if _, ok := lite.BatteryHours[PerfRage]; ok {
		t.Fatal("PAW3311 should not publish a rage hour range")
	}
	pro := LookupModel(3395, "")
	if pro.BatteryHours[PerfPerformance] != "80-95" || pro.Sensor != "PAW3950" {
		t.Fatalf("PAW3950 fallback: %+v", pro.BatteryHours)
	}
	screen := LookupModel(3572, "")
	if screen.BatteryHours[PerfPerformance] != "88-95" {
		t.Fatalf("id 3572 hours: %+v", screen.BatteryHours)
	}
	hz, fps, err := PerfRage.Preset()
	if err != nil || hz != 8000 || !fps || PerfRage.Label() != "狂暴模式" {
		t.Fatalf("rage preset hz=%d fps=%v err=%v label=%s", hz, fps, err, PerfRage.Label())
	}
	hz, fps, err = PerfOffice.Preset()
	if err != nil || hz != 500 || fps {
		t.Fatalf("office preset hz=%d fps=%v err=%v", hz, fps, err)
	}
}

func TestKeyActionRoundTrip(t *testing.T) {
	samples := [][4]byte{
		{1, 0, 240, 0},
		{1, 0, 241, 0},
		{1, 0, 242, 0},
		{1, 0, 244, 0},
		{1, 0, 243, 0},
		{1, 0, 245, 1},
		{1, 0, 245, 255},
		{1, 0, 245, 0},
		{1, 0, 246, 0},
		{20, 0, 0, 0},
		{20, 0, 1, 0},
		{20, 0, 2, 0},
		{20, 0, 4, 0},
		{8, 0, 3, 0},
		{8, 0, 1, 0},
		{8, 0, 2, 0},
		{0, 0, 0, 0},
		{0, 224, 6, 0},
		{0, 224, 225, 4},
		{11, 240, 20, 0},
		{11, 4, 30, 5},
		{9, 0, 3, 0},
		{9, 1, 15, 0},
		{9, 2, 1, 0},
		{3, 0, 233, 0},
		{3, 0, 234, 0},
		{3, 0, 226, 0},
		{3, 0, 205, 0},
		{3, 0, 182, 0},
		{3, 0, 181, 0},
		{3, 0, 183, 0},
		{3, 0, 131, 1},
		{3, 0, 146, 1},
		{3, 0, 148, 1},
		{3, 0, 138, 1},
		{3, 0, 35, 2},
		{3, 0, 33, 2},
		{3, 0, 36, 2},
		{3, 0, 39, 2},
		{3, 0, 111, 0},
		{3, 0, 112, 0},
		{0, 0, 227, 15},
		{0, 0, 227, 7},
		{0, 0, 70, 0},
		{0, 0, 224, 6},
		{0, 0, 224, 25},
		{0, 0, 224, 27},
		{2, 0, 130, 0},
	}
	for _, code := range samples {
		got := EncodeKeyAction(DecodeKeyAction(code))
		if got != code {
			t.Fatalf("roundtrip %v -> %v (%s)", code, got, DecodeKeyAction(code).Kind)
		}
	}

	legacy := DecodeKeyAction([4]byte{0, 2, 0, 20})
	if legacy.Kind != KeyKindMouse || legacy.Name != "DPI-" || EncodeKeyAction(legacy) != [4]byte{20, 0, 2, 0} {
		t.Fatalf("legacy DPI-: %+v encoded %v", legacy, EncodeKeyAction(legacy))
	}
	if LabelForKeyCode([4]byte{0, 2, 0, 20}) != "DPI -" || LabelForKeyCode([4]byte{20, 0, 2, 0}) != "DPI -" {
		t.Fatal("DPI- labels drifted")
	}

	fire := FireKey(0, 15, 0)
	if fire.Code != [4]byte{11, 240, 15, 0} {
		t.Fatalf("default fire target: %v", fire.Code)
	}
	combo := ComboKeys(ModCtrl, ModShift, 4)
	if combo.Code != [4]byte{0, ModCtrl, ModShift, 4} {
		t.Fatalf("combo: %v", combo.Code)
	}
	code, ok := HIDKeyCode("C")
	if !ok || code != 6 {
		t.Fatalf("HID C = %d ok=%v", code, ok)
	}
	name, ok := HIDKeyName(41)
	if !ok || name != "Esc" {
		t.Fatalf("41 name = %q", name)
	}
	if c, ok := HIDKeyCode("Escape"); !ok || c != 41 {
		t.Fatalf("Escape alias = %d", c)
	}
	for _, key := range HIDKeys {
		got, ok := HIDKeyCode(key.Name)
		if !ok || got != key.Code {
			t.Fatalf("name %s -> %d want %d", key.Name, got, key.Code)
		}
		if back, ok := HIDKeyName(key.Code); !ok || back == "" {
			t.Fatalf("code %d has no name", key.Code)
		}
	}
}
