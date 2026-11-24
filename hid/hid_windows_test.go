//go:build windows

package hid

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestWindowsHIDStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(winGUID{}); got != 16 {
		t.Fatalf("winGUID size = %d, want 16", got)
	}
	if got := unsafe.Sizeof(hiddAttributes{}); got != 12 {
		t.Fatalf("hiddAttributes size = %d, want 12", got)
	}
	if got := unsafe.Sizeof(hidpCaps{}); got != 64 {
		t.Fatalf("hidpCaps size = %d, want 64", got)
	}
	wantDID := uintptr(28)
	wantDetail := uint32(6)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		wantDID = 32
		wantDetail = 8
	}
	if got := unsafe.Sizeof(spDeviceInterfaceData{}); got != wantDID {
		t.Fatalf("spDeviceInterfaceData size = %d, want %d", got, wantDID)
	}
	if got := detailDataHeaderSize(); got != wantDetail {
		t.Fatalf("detailDataHeaderSize() = %d, want %d", got, wantDetail)
	}
}

func TestDecodeUTF16Bytes(t *testing.T) {
	src := []uint16{'V', 'K', ' ', 'M', '5', 0, 'X'}
	raw := make([]byte, len(src)*2)
	for i, v := range src {
		binary.LittleEndian.PutUint16(raw[i*2:i*2+2], v)
	}
	if got := decodeUTF16Bytes(raw); got != "VK M5" {
		t.Fatalf("decodeUTF16Bytes = %q, want %q", got, "VK M5")
	}
}
