package hid

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestMacroKeyboardRoundTrip(t *testing.T) {
	src := Macro{
		RepeatCount: 3,
		Events: []MacroEvent{
			{Type: MacroKeyboard, Action: MacroActionDown, KeyCode: 4, DelayMs: 10},
			{Type: MacroKeyboard, Action: MacroActionUp, KeyCode: 4, DelayMs: 200},
			{Type: MacroMouseButton, Action: MacroActionDown, KeyCode: 240, DelayMs: 8},
			{Type: MacroMouseButton, Action: MacroActionUp, KeyCode: 240, DelayMs: 12},
		},
	}
	buf, err := EncodeMacroBuffer(src)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(buf[0:2]) != 3 {
		t.Fatalf("repeat = %d", binary.LittleEndian.Uint16(buf[0:2]))
	}
	// A down, delay 10: [4, 138]
	if buf[2] != 4 || buf[3] != 138 {
		t.Fatalf("short down: %v", buf[2:4])
	}
	// A up, delay 200: [4, 0, 200, 0]
	if buf[4] != 4 || buf[5] != 0 || binary.LittleEndian.Uint16(buf[6:8]) != 200 {
		t.Fatalf("long up: %v", buf[4:8])
	}
	got := DecodeMacroBuffer(buf[:])
	if got.RepeatCount != 3 || len(got.Events) != 8 {
		t.Fatalf("decoded %+v", got)
	}
	if got.Events[0].Type != MacroKeyboard || got.Events[0].Action != MacroActionDown || got.Events[0].KeyCode != 4 {
		t.Fatalf("event0 %+v", got.Events[0])
	}
	if got.Events[1].Type != MacroDelay || got.Events[1].DelayMs != 10 {
		t.Fatalf("event1 %+v", got.Events[1])
	}
	if got.Events[3].DelayMs != 200 {
		t.Fatalf("long delay %+v", got.Events[3])
	}
	again, err := EncodeMacroBuffer(Macro{RepeatCount: got.RepeatCount, Events: got.Events})
	if err != nil {
		t.Fatal(err)
	}
	if again != buf {
		t.Fatal("re-encode drifted")
	}
}

func TestMacroMouseMoveOfficialDelay(t *testing.T) {
	src := Macro{Events: []MacroEvent{{
		Type: MacroMouseMove, DX: 5, DY: -3, DelayMs: 10,
	}}}
	buf, err := EncodeMacroBuffer(src)
	if err != nil {
		t.Fatal(err)
	}
	if buf[2] != 249 || buf[3] != 10 || int8(buf[4]) != 5 || int8(buf[5]) != -3 {
		t.Fatalf("move bytes %v", buf[2:6])
	}
	got := DecodeMacroBuffer(buf[:])
	if len(got.Events) != 2 || got.Events[0].DX != 5 || got.Events[0].DY != -3 {
		t.Fatalf("move decode %+v", got.Events)
	}
	// buffToMacroEvents stores a 1-byte mouse-move delay and reads it as value>>1.
	if got.Events[1].DelayMs != 5 {
		t.Fatalf("official short move delay = %d, want 5", got.Events[1].DelayMs)
	}

	long := Macro{Events: []MacroEvent{{Type: MacroMouseMove, DX: -1, DY: 2, DelayMs: 300}}}
	lbuf, err := EncodeMacroBuffer(long)
	if err != nil {
		t.Fatal(err)
	}
	if lbuf[3] != 0 || binary.LittleEndian.Uint16(lbuf[6:8]) != 300 {
		t.Fatalf("long move %v", lbuf[2:8])
	}
	lgot := DecodeMacroBuffer(lbuf[:])
	if len(lgot.Events) != 2 || lgot.Events[1].DelayMs != 300 || lgot.Events[0].DX != -1 {
		t.Fatalf("long move decode %+v", lgot.Events)
	}
}

func TestMacroProtocolPackets(t *testing.T) {
	tr := newRealVKM5Mock()
	resp := make([]byte, 64)
	binary.LittleEndian.PutUint16(resp[0:2], 2)
	resp[2] = 4
	resp[3] = 138
	tr.responses[CmdGetMacro] = resp

	client := NewClient(tr)
	client.SetVendorSleep(0)
	m, err := client.ReadMacro(3)
	if err != nil {
		t.Fatal(err)
	}
	if m.Slot != 3 || m.RepeatCount != 2 || len(m.Events) != 2 {
		t.Fatalf("read macro %+v", m)
	}
	if tr.sent[len(tr.sent)-1][0] != CmdGetMacro || !tr.expect[len(tr.expect)-1] {
		t.Fatal("get macro must be a read")
	}
	// Raw 0x83 body does not echo the opcode; the decoder still accepts it.
	if responseOmitsCommandEcho(CmdGetMacro) != true || responseOmitsCommandEcho(CmdGetKeyMatrix) != true || responseOmitsCommandEcho(CmdGetGun) != true {
		t.Fatal("raw response classification")
	}
	if responseOmitsCommandEcho(CmdGetVersionID) {
		t.Fatal("version command echoes")
	}

	before := len(tr.sent)
	if err := client.WriteMacro(3, Macro{
		RepeatCount: 2,
		Events:      []MacroEvent{{Type: MacroKeyboard, Action: MacroActionDown, KeyCode: 4, DelayMs: 10}},
	}); err != nil {
		t.Fatal(err)
	}
	pkts := tr.sent[before:]
	if len(pkts) != 1 {
		t.Fatalf("packets = %d, want 1", len(pkts))
	}
	p := pkts[0]
	if p[0] != CmdSetMacro || p[1] != 3 || p[2] != 0 || p[3] != 56 || p[4] != 1 {
		t.Fatalf("macro header %v", p[:8])
	}
	if p[8] != 2 || p[10] != 4 || p[11] != 138 {
		t.Fatalf("macro payload %v", p[8:14])
	}
	if tr.expect[len(tr.expect)-1] {
		t.Fatal("macro write must not wait for a reply")
	}
}

func TestWritesNeverExpectReply(t *testing.T) {
	tr := newRealVKM5Mock()
	client := NewClient(tr)
	if client.vendorSleep != 100*time.Millisecond {
		t.Fatalf("BT settle = %s", client.vendorSleep)
	}
	client.SetVendorSleep(0)

	if err := client.SetMotionSync(true); err != nil {
		t.Fatal(err)
	}
	if tr.expect[len(tr.expect)-1] {
		t.Fatal("BT Set_* waited for a reply")
	}
	if tr.sent[len(tr.sent)-1][0] != CmdSetMotionSync {
		t.Fatal("motion sync opcode")
	}

	tr.info.Mode = ModeUSB
	usb := NewClient(tr)
	if usb.vendorSleep != 40*time.Millisecond {
		t.Fatalf("USB settle = %s", usb.vendorSleep)
	}

	stages := make([]DPIStage, 6)
	for i := range stages {
		stages[i] = DPIStage{X: uint16(400 * (i + 1)), Y: uint16(400 * (i + 1)), R: 1}
	}
	if err := client.SetDPI(1, DPIConfig{Current: 3, Count: 4, Stages: stages}); err != nil {
		t.Fatal(err)
	}
	dpi := tr.sent[len(tr.sent)-1]
	if dpi[3] != 4 || dpi[2] != 3 {
		t.Fatalf("dpi count header %v", dpi[:4])
	}
	if binary.LittleEndian.Uint16(dpi[16:18]) != 0 {
		t.Fatal("stage past count was encoded")
	}

	matrix := DefaultMatrix()
	if err := client.SetAllKeys(2, matrix); err != nil {
		t.Fatal(err)
	}
	a := tr.sent[len(tr.sent)-2]
	b := tr.sent[len(tr.sent)-1]
	if a[0] != CmdSetAllKeys || a[1] != 2 || a[2] != 0 || b[2] != 1 {
		t.Fatalf("all-keys headers %v / %v", a[:4], b[:4])
	}
	if a[8] != 1 || a[10] != 240 || b[8] != 1 || b[10] != 245 || b[15] != 255 {
		t.Fatalf("all-keys payload a=%v b=%v", a[8:16], b[8:16])
	}

	if err := client.ApplyPerformanceMode(PerfGaming); err != nil {
		t.Fatal(err)
	}
	rate := tr.sent[len(tr.sent)-2]
	fps := tr.sent[len(tr.sent)-1]
	code, _ := EncodeReportRate(4000)
	if rate[0] != CmdSetReportRate || rate[1] != code || fps[0] != CmdSetFPS20000 || fps[1] != 0 {
		t.Fatalf("gaming preset rate=%v fps=%v", rate[:2], fps[:2])
	}
	if tr.expect[len(tr.expect)-1] || tr.expect[len(tr.expect)-2] {
		t.Fatal("performance writes waited for replies")
	}

	onResp := make([]byte, 64)
	onResp[0] = CmdGetTrue8K
	onResp[1] = 1
	tr.responses[CmdGetTrue8K] = onResp
	on, err := client.ReadTrue8K()
	if err != nil || !on {
		t.Fatalf("true8k on=%v err=%v", on, err)
	}
	if err := client.SetTrue8K(false); err != nil {
		t.Fatal(err)
	}
	if tr.sent[len(tr.sent)-1][0] != CmdSetTrue8K || tr.sent[len(tr.sent)-1][1] != 0 || tr.expect[len(tr.expect)-1] {
		t.Fatal("set true8k frame")
	}
}
