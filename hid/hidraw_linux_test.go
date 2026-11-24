//go:build linux

package hid

import (
	"syscall"
	"testing"
	"time"
)

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
