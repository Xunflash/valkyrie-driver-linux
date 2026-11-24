package hid

import (
	"encoding/binary"
	"testing"
)

type mockTransport struct {
	info      DeviceInfo
	battery   int
	fw        uint16
	responses map[byte][]byte
	sent      [][]byte
	expect    []bool
}

func (m *mockTransport) DeviceInfo() DeviceInfo { return m.info }
func (m *mockTransport) Close() error           { return nil }
func (m *mockTransport) ReadBattery() (int, uint16, error) {
	return m.battery, m.fw, nil
}
func (m *mockTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	cp := append([]byte(nil), cmd...)
	m.sent = append(m.sent, cp)
	m.expect = append(m.expect, expectReply)
	if !expectReply {
		return nil, nil
	}
	if resp, ok := m.responses[cmd[0]]; ok {
		out := make([]byte, 64)
		copy(out, resp)
		return out, nil
	}
	// Echo command back for write ACKs over BT
	return cp, nil
}

func newRealVKM5Mock() *mockTransport {
	// Real captured response frames from user's VK M5 (/dev/hidraw4, ID=2914, fw=v0810, 88% batt)
	verResp := make([]byte, 64)
	verResp[0] = CmdGetVersionID
	binary.LittleEndian.PutUint32(verResp[1:5], 2914)
	binary.LittleEndian.PutUint16(verResp[5:7], 0x0810)

	allParams := make([]byte, 64)
	allParams[0] = CmdGetAllParams0
	allParams[8] = 0 // profile 0
	allParams[9] = 3 // debounce 3ms
	allParams[10] = 3
	binary.LittleEndian.PutUint16(allParams[11:13], 120) // 2.4G sleep 120s
	binary.LittleEndian.PutUint16(allParams[13:15], 120) // BT sleep 120s
	allParams[15] = 2                                    // 500 Hz
	allParams[16] = 0                                    // LOD 1mm
	allParams[17] = 0                                    // lineRepair off
	allParams[18] = 0                                    // waveRepair off
	allParams[19] = 1                                    // motionSync on
	allParams[28] = 0                                    // FPS20000 off
	allParams[39] = 0                                    // angle 0
	allParams[40] = 1                                    // lowLatency on
	allParams[41] = 0                                    // true8k off

	dpiResp := make([]byte, 64)
	dpiResp[0] = CmdGetDPI
	dpiResp[2] = 2 // stage 3 active (1200 DPI)
	dpiResp[3] = 6
	dpis := []uint16{400, 800, 1200, 1600, 3200, 5200}
	colors := [][3]byte{
		{255, 0, 0}, {0, 255, 0}, {0, 0, 255},
		{0, 255, 255}, {255, 255, 0}, {255, 0, 255},
	}
	for i, d := range dpis {
		binary.LittleEndian.PutUint16(dpiResp[8+i*2:10+i*2], d)
		binary.LittleEndian.PutUint16(dpiResp[24+i*2:26+i*2], d)
		dpiResp[40+i*3] = colors[i][0]
		dpiResp[41+i*3] = colors[i][1]
		dpiResp[42+i*3] = colors[i][2]
	}

	keyResp := make([]byte, 64)
	for _, slot := range ConfigurableSlots {
		copy(keyResp[int(slot.Index)*4:int(slot.Index)*4+4], slot.DefaultAction[:])
	}

	angResp := make([]byte, 64)
	angResp[0] = CmdGetSensorAngle
	angResp[1] = 0

	return &mockTransport{
		info: DeviceInfo{
			Path: "/dev/hidraw4",
			VID:  VIDValkyrie,
			PID:  PIDVKM5BT,
			Name: "VK M5",
			Mode: ModeBT,
		},
		battery: 88,
		fw:      0x0810,
		responses: map[byte][]byte{
			CmdGetVersionID:   verResp,
			CmdGetAllParams0:  allParams,
			CmdGetDPI:         dpiResp,
			CmdGetKeyMatrix:   keyResp,
			CmdGetSensorAngle: angResp,
		},
	}
}

func TestBit7Checksum(t *testing.T) {
	f := EncodeBit7([]byte{CmdGetVersionID})
	if len(f) != 64 {
		t.Fatalf("expected 64 bytes, got %d", len(f))
	}
	want := byte(255 - CmdGetVersionID)
	if f[7] != want {
		t.Fatalf("checksum = 0x%02x, want 0x%02x", f[7], want)
	}

	f2 := EncodeBit7([]byte{CmdSetSleepBT, 0x78, 0x00})
	want2 := byte(255 - ((uint32(CmdSetSleepBT) + 0x78) & 0xFF))
	if f2[7] != want2 {
		t.Fatalf("checksum2 = 0x%02x, want 0x%02x", f2[7], want2)
	}
}

func TestReadAllRealVKM5Snapshot(t *testing.T) {
	tr := newRealVKM5Mock()
	client := NewClient(tr)
	client.SetVendorSleep(0)

	st, err := client.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if st.DevID != 2914 || st.Spec.Model != "VK M5" || st.Spec.Sensor != "PAW3395" {
		t.Fatalf("unexpected identity: id=%d model=%s sensor=%s", st.DevID, st.Spec.Model, st.Spec.Sensor)
	}
	if st.FirmwareString() != "v0810" || !st.SupportsLowLatency() {
		t.Fatalf("unexpected fw: %s supportsLowLatency=%v", st.FirmwareString(), st.SupportsLowLatency())
	}
	if st.Battery != 88 {
		t.Fatalf("battery = %d, want 88", st.Battery)
	}
	if st.Params.ReportRate != 500 || !st.Params.MotionSync || !st.Params.LowLatency || st.Params.Debounce != 3 {
		t.Fatalf("unexpected params: %+v", st.Params)
	}
	if st.DPI.Current != 2 || len(st.DPI.Stages) != 6 || st.DPI.Stages[2].X != 1200 || st.DPI.Stages[2].B != 255 {
		t.Fatalf("unexpected DPI: %+v", st.DPI)
	}
	if LabelForKeyCode(st.Keys[5]) != "DPI 循环切换" {
		t.Fatalf("unexpected DPI key label: %s", LabelForKeyCode(st.Keys[5]))
	}
	if st.Keys[0] != [4]byte{1, 0, 240, 0} || st.Keys[14] != [4]byte{1, 0, 245, 1} {
		t.Fatalf("key matrix slots not placed by index: %+v", st.Keys)
	}
	for i, exp := range tr.expect {
		if !exp {
			t.Fatalf("read command %d must expect a reply, frame %x", i, tr.sent[i][:4])
		}
	}
}

func TestWriteCommandsEncoding(t *testing.T) {
	tr := newRealVKM5Mock()
	client := NewClient(tr)
	client.SetVendorSleep(0)

	// 1. SetDPI with all-zero RGB guard
	err := client.SetDPI(0, DPIConfig{
		Current: 1,
		Count:   2,
		Stages: []DPIStage{
			{X: 800, Y: 800, R: 0, G: 0, B: 0},
			{X: 1600, Y: 1800, R: 0, G: 0, B: 0},
		},
	})
	if err != nil {
		t.Fatalf("SetDPI error: %v", err)
	}
	last := tr.sent[len(tr.sent)-1]
	if last[0] != CmdSetDPI || last[2] != 1 || last[3] != 2 {
		t.Fatalf("bad SetDPI header: %x", last[:8])
	}
	if binary.LittleEndian.Uint16(last[10:12]) != 1600 || binary.LittleEndian.Uint16(last[26:28]) != 1800 {
		t.Fatalf("bad X/Y DPI encoding")
	}
	if last[42] == 0 {
		t.Fatalf("all-zero RGB guard did not trigger")
	}

	// 2. Negative sensor angle (-15 deg -> 241 == 0xF1)
	if err := client.SetSensorAngle(-15); err != nil {
		t.Fatalf("SetSensorAngle error: %v", err)
	}
	ang := tr.sent[len(tr.sent)-1]
	if ang[0] != CmdSetSensorAngle || ang[1] != 241 || DecodeAngle(ang[1]) != -15 {
		t.Fatalf("bad angle frame: cmd=%d val=%d decoded=%d", ang[0], ang[1], DecodeAngle(ang[1]))
	}

	// 3. Key remap
	code, ok := KeyCodeForLabel("复制 (Ctrl+C)")
	if !ok {
		t.Fatal("preset not found")
	}
	if err := client.SetKeySimple(4, code); err != nil {
		t.Fatalf("SetKeySimple error: %v", err)
	}
	kp := tr.sent[len(tr.sent)-1]
	if kp[0] != CmdSetKeySimple || kp[1] != 4 || kp[10] != 224 || kp[11] != 6 {
		t.Fatalf("bad key remap packet: %v", kp[:12])
	}

	// 4. Polling rate encoding roundtrip
	for _, hz := range SupportedReportRates {
		wire, err := EncodeReportRate(hz)
		if err != nil {
			t.Fatalf("EncodeReportRate(%d): %v", hz, err)
		}
		if got := DecodeReportRate(wire); got != hz {
			t.Fatalf("roundtrip %d -> %d -> %d", hz, wire, got)
		}
	}
}

func TestParseReportDescriptor(t *testing.T) {
	// Simulated BLE vendor collection (UsagePage 0xFF55, ReportID 6, Input)
	btDesc := []byte{
		0x06, 0x55, 0xFF, // Usage Page (0xFF55)
		0x0A, 0x02, 0x02, // Usage (0x0202)
		0xA1, 0x01, // Collection (Application)
		0x85, 0x06, // Report ID (6)
		0x75, 0x08, // Report Size (8)
		0x95, 0x41, // Report Count (65)
		0x81, 0x02, // Input (Data, Var, Abs)
		0xC0, // End Collection
	}
	meta := ParseReportDescriptor(btDesc)
	if !meta.HasVendorBT || meta.VendorPage != 0xFF55 || meta.VendorReport != 6 {
		t.Fatalf("failed to detect BT vendor collection: %+v", meta)
	}

	// Standard mouse collection (should NOT match)
	mouseDesc := []byte{
		0x05, 0x01, // Usage Page (Generic Desktop)
		0x09, 0x02, // Usage (Mouse)
		0xA1, 0x01,
		0x81, 0x02,
		0xC0,
	}
	meta2 := ParseReportDescriptor(mouseDesc)
	if meta2.HasVendorBT || meta2.HasVendorUSB {
		t.Fatalf("standard mouse incorrectly matched as vendor: %+v", meta2)
	}

	// USB 0xFFFF vendor collection
	usbDesc := []byte{
		0x06, 0xFF, 0xFF, // Usage Page (0xFFFF)
		0x09, 0x01,
		0xA1, 0x01,
		0xB1, 0x02, // Feature
		0xC0,
	}
	meta3 := ParseReportDescriptor(usbDesc)
	if !meta3.HasVendorUSB || meta3.VendorPage != 0xFFFF {
		t.Fatalf("failed to detect USB vendor collection: %+v", meta3)
	}
}
