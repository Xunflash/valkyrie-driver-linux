package hid

import (
	"testing"
	"time"
)

func TestSplitBTReportRawPayload(t *testing.T) {
	raw := make([]byte, 66)
	raw[0] = btReportID
	raw[1] = btCmdPrefix
	raw[2] = 1 // key-matrix data, not the 0x81 opcode
	raw[3] = 0
	raw[4] = 240
	out, ok := splitBTReport(raw, len(raw), btCmdPrefix)
	if !ok || out[0] != 1 || out[2] != 240 {
		t.Fatalf("framed payload ok=%v out=%v", ok, out[:4])
	}

	bare := []byte{btCmdPrefix, 0x04, 0x8A}
	out, ok = splitBTReport(bare, len(bare), btCmdPrefix)
	if !ok || out[0] != 0x04 || out[1] != 0x8A {
		t.Fatalf("prefix-only payload ok=%v out=%v", ok, out[:2])
	}
	if _, ok := splitBTReport([]byte{btReportID, btStatPrefix, 80}, 3, btCmdPrefix); ok {
		t.Fatal("status prefix matched a command read")
	}
}

func TestBTWriteGap(t *testing.T) {
	if d := btWriteDelay(time.Time{}, btMinWriteGap); d != 0 {
		t.Fatalf("first gap = %s", d)
	}
	last := time.Now().Add(-20 * time.Millisecond)
	d := btWriteDelay(last, btMinWriteGap)
	if d < 30*time.Millisecond || d > 50*time.Millisecond {
		t.Fatalf("gap = %s", d)
	}
	if d := btWriteDelay(time.Now().Add(-80*time.Millisecond), btMinWriteGap); d != 0 {
		t.Fatalf("stale gap = %s", d)
	}
}

func TestSortDevicesByPriority(t *testing.T) {
	devs := []DeviceInfo{
		{Path: "bt", Mode: ModeBT},
		{Path: "24g", Mode: Mode24G},
		{Path: "usb", Mode: ModeUSB},
	}
	sortDevicesByPriority(devs)
	if devs[0].Mode != ModeUSB || devs[1].Mode != Mode24G || devs[2].Mode != ModeBT {
		t.Fatalf("unexpected priority order: %+v", devs)
	}
}

type fakeFeatureConn struct {
	sent      [][]byte
	responses [][]byte
	idx       int
}

func (f *fakeFeatureConn) SendFeature(data []byte) error {
	cp := append([]byte(nil), data...)
	f.sent = append(f.sent, cp)
	return nil
}

func (f *fakeFeatureConn) GetFeature() ([]byte, error) {
	if f.idx >= len(f.responses) {
		return make([]byte, 64), nil
	}
	r := f.responses[f.idx]
	f.idx++
	return r, nil
}

func (f *fakeFeatureConn) Close() error { return nil }

func TestDongle24GHandshake(t *testing.T) {
	f7Ready := make([]byte, 64)
	f7Ready[0] = 1  // canRead
	f7Ready[2] = 76 // battery 76%
	f7Ready[4] = 0  // online
	f7Ready[5] = 1  // canSend
	f7Ready[6] = 2  // mouse mask

	payloadResp := make([]byte, 64)
	payloadResp[0] = CmdGetVersionID
	payloadResp[1] = 0x62
	payloadResp[2] = 0x0B

	fc := &fakeFeatureConn{
		responses: [][]byte{f7Ready, f7Ready, payloadResp},
	}
	tr := &dongle24GTransport{
		conn:      fc,
		info:      DeviceInfo{Mode: Mode24G, Path: "mock-24g"},
		pollDelay: -1,
	}

	got, err := tr.Exchange(EncodeBit7([]byte{CmdGetVersionID}), true)
	if err != nil {
		t.Fatalf("2.4G Exchange failed: %v", err)
	}
	if got[0] != CmdGetVersionID || got[1] != 0x62 {
		t.Fatalf("unexpected 2.4G response: %x", got[:4])
	}
	if len(fc.sent) != 5 || fc.sent[0][0] != 0xF7 || fc.sent[1][0] != 0xF6 || fc.sent[2][0] != CmdGetVersionID || fc.sent[3][0] != 0xF7 || fc.sent[4][0] != 0xFC {
		t.Fatalf("unexpected 2.4G command sequence: %v", fc.sent)
	}
}
