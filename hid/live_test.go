//go:build linux

package hid

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// TestLiveIdempotentBTWrite reads /dev/hidraw4 and writes the current MotionSync
// value back unchanged. It refuses to run unless VK_LIVE_HID=1, and it never
// resets the mouse or touches DPI, keys, or any other setting.
func TestLiveIdempotentBTWrite(t *testing.T) {
	if os.Getenv("VK_LIVE_HID") != "1" {
		t.Skip("set VK_LIVE_HID=1 to run the non-destructive /dev/hidraw4 check")
	}
	devs, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	wantPath := os.Getenv("VK_LIVE_HIDRAW")
	if wantPath == "" {
		wantPath = "/dev/hidraw4"
	}
	var info DeviceInfo
	found := false
	for _, dev := range devs {
		if dev.Path == wantPath {
			info = dev
			found = true
			break
		}
	}
	if !found {
		for _, dev := range devs {
			if dev.Mode == ModeBT {
				info = dev
				found = true
				t.Logf("%s is not a Valkyrie interface; using discovered BT device %s", wantPath, dev.Path)
				break
			}
		}
	}
	if !found {
		t.Fatalf("no Valkyrie BT interface (looked for %s, discovered %d)", wantPath, len(devs))
	}
	tr, err := OpenTransport(info)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(tr)
	defer client.Close()

	before, err := client.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before mode=%s motion=%v debounce=%d rate=%d dpi=%+v",
		before.Info.Mode, before.Params.MotionSync, before.Params.Debounce, before.Params.ReportRate, before.DPI)

	start := time.Now()
	if err := client.SetMotionSync(before.Params.MotionSync); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	t.Logf("idempotent SetMotionSync(%v) took %s", before.Params.MotionSync, elapsed)
	if elapsed > 800*time.Millisecond {
		t.Fatalf("write took %s; a Bluetooth reply wait would be about 1.3s", elapsed)
	}

	dpiStart := time.Now()
	if err := client.SetDPI(before.Params.Profile, before.DPI); err != nil {
		t.Fatalf("idempotent SetDPI failed: %v", err)
	}
	t.Logf("idempotent SetDPI(cur=%d, x=%d) with live readback verify took %s",
		before.DPI.Current, before.DPI.Stages[before.DPI.Current].X, time.Since(dpiStart))

	after, err := client.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if after.Params.MotionSync != before.Params.MotionSync ||
		after.Params.Debounce != before.Params.Debounce ||
		after.Params.ReportRate != before.Params.ReportRate ||
		after.Params.ScrollUpTime != before.Params.ScrollUpTime ||
		after.Params.SilentHeight != before.Params.SilentHeight ||
		after.Params.LineRepair != before.Params.LineRepair ||
		after.Params.WaveRepair != before.Params.WaveRepair ||
		after.Params.FPS20000 != before.Params.FPS20000 ||
		after.Params.AngleTune != before.Params.AngleTune ||
		after.Params.LowLatency != before.Params.LowLatency ||
		after.Params.True8K != before.Params.True8K ||
		after.Params.Sleep24G != before.Params.Sleep24G ||
		after.Params.SleepBT != before.Params.SleepBT ||
		after.Params.Profile != before.Params.Profile {
		t.Fatalf("params changed\nbefore=%+v\nafter=%+v", before.Params, after.Params)
	}
	if after.DPI.Current != before.DPI.Current || len(after.DPI.Stages) != len(before.DPI.Stages) {
		t.Fatalf("DPI changed: before=%+v after=%+v", before.DPI, after.DPI)
	}
	for i := range before.DPI.Stages {
		if after.DPI.Stages[i] != before.DPI.Stages[i] {
			t.Fatalf("DPI stage %d changed: %+v -> %+v", i, before.DPI.Stages[i], after.DPI.Stages[i])
		}
	}
	if !bytes.Equal(bytesFlatten(before.Keys), bytesFlatten(after.Keys)) {
		t.Fatalf("keys changed\nbefore=%v\nafter=%v", before.Keys, after.Keys)
	}
}

func bytesFlatten(keys [16][4]byte) []byte {
	out := make([]byte, 0, 64)
	for _, row := range keys {
		out = append(out, row[:]...)
	}
	return out
}
