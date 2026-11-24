//go:build linux

package hid

import (
	"syscall"
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

func TestBTWriteDoesNotWaitForReply(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fds[0])
	defer syscall.Close(fds[1])
	// Production BT hidraw is opened O_NONBLOCK; drain() depends on EAGAIN.
	if err := syscall.SetNonblock(fds[0], true); err != nil {
		t.Fatal(err)
	}

	tr := &btTransport{fd: fds[0], info: DeviceInfo{Mode: ModeBT, Path: "socketpair"}}
	payload := make([]byte, 64)
	payload[0] = CmdSetMotionSync
	payload[1] = 1

	start := time.Now()
	if _, err := tr.Exchange(payload, false); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 30*time.Millisecond {
		t.Fatalf("first write blocked for %s", time.Since(start))
	}
	second := time.Now()
	if _, err := tr.Exchange(payload, false); err != nil {
		t.Fatal(err)
	}
	gap := time.Since(second)
	if gap < 50*time.Millisecond || gap > 200*time.Millisecond {
		t.Fatalf("write gap = %s, want about 60ms and not a 450ms reply wait", gap)
	}
}
