package hid

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	btReportID   byte = 0x06
	btCmdPrefix  byte = 0x55
	btStatPrefix byte = 0x77
	btPacketLen       = 66 // 1B reportID + 1B prefix + 64B payload

	// btMinWriteGap is the minimum spacing between BLE GATT writes.
	// The official driver uses SEND_TIME=100 ms before each BT sendMsg; 60 ms
	// is enough to stop back-to-back writes from dropping on this firmware.
	btMinWriteGap = 60 * time.Millisecond
	// btReadWait is how long a Get_* waits for [0x06, 0x55, payload] before retrying.
	btReadWait = 500 * time.Millisecond
)

// ReportMeta summarizes vendor-specific collections found in a HID report descriptor.
type ReportMeta struct {
	HasVendorBT   bool   // UsagePage 0xFF55 with ReportID 6
	HasVendorUSB  bool   // UsagePage 0xFFFF with Feature/Vendor report
	VendorPage    uint16 // 0xFF55 or 0xFFFF
	VendorReport  byte
	RawUsagePages []uint16
}

// ParseReportDescriptor parses a raw HID report descriptor to identify Valkyrie vendor collections.
func ParseReportDescriptor(desc []byte) ReportMeta {
	var meta ReportMeta
	var curPage uint16
	var curReportID byte
	seenPages := make(map[uint16]bool)

	for i := 0; i < len(desc); {
		b := desc[i]
		if b == 0xFE { // Long item
			if i+1 >= len(desc) {
				break
			}
			i += 3 + int(desc[i+1])
			continue
		}
		sizeCode := b & 0x03
		size := int(sizeCode)
		if sizeCode == 3 {
			size = 4
		}
		itemType := (b >> 2) & 0x03
		tag := (b >> 4) & 0x0F

		if i+1+size > len(desc) {
			break
		}
		var val uint32
		for j := 0; j < size; j++ {
			val |= uint32(desc[i+1+j]) << (8 * j)
		}

		// Global items (itemType == 1)
		if itemType == 1 {
			switch tag {
			case 0x0: // Usage Page
				curPage = uint16(val)
				if !seenPages[curPage] {
					seenPages[curPage] = true
					meta.RawUsagePages = append(meta.RawUsagePages, curPage)
				}
				if curPage == 0xFFFF {
					meta.HasVendorUSB = true
					if meta.VendorPage == 0 {
						meta.VendorPage = 0xFFFF
					}
				}
			case 0x8: // Report ID
				curReportID = byte(val)
				if curPage == 0xFF55 && curReportID == btReportID {
					meta.HasVendorBT = true
					meta.VendorPage = 0xFF55
					meta.VendorReport = btReportID
				}
			}
		}
		// Main items (itemType == 0): Input(8), Output(9), Feature(11)
		if itemType == 0 && (tag == 8 || tag == 9 || tag == 11) {
			if curPage == 0xFF55 && curReportID == btReportID {
				meta.HasVendorBT = true
				meta.VendorPage = 0xFF55
				meta.VendorReport = btReportID
			} else if curPage == 0xFFFF {
				meta.HasVendorUSB = true
				if meta.VendorPage == 0 {
					meta.VendorPage = 0xFFFF
					meta.VendorReport = curReportID
				}
			}
		}
		i += 1 + size
	}
	return meta
}

func isSupportedVIDPID(vid, pid uint16, name string) bool {
	if vid == VIDValkyrie {
		return true
	}
	if vid == VIDRongYuan {
		low := strings.ToLower(name)
		if strings.Contains(low, "vk") || strings.Contains(low, "valkyrie") || strings.Contains(low, "mouse") || pid == PIDVKM5USB || pid == PIDVKM524G || pid == PIDVKM5BT {
			return true
		}
	}
	return false
}

func is24GDongle(pid uint16, name string) bool {
	if pid == PIDVKM524G {
		return true
	}
	low := strings.ToLower(name)
	return strings.Contains(low, "2.4g") || strings.Contains(low, "dongle") || strings.Contains(low, "receiver")
}

func sortDevicesByPriority(found []DeviceInfo) {
	priority := map[ConnectionMode]int{ModeUSB: 0, Mode24G: 1, ModeBT: 2}
	sort.Slice(found, func(i, j int) bool {
		if priority[found[i].Mode] != priority[found[j].Mode] {
			return priority[found[i].Mode] < priority[found[j].Mode]
		}
		return found[i].Path < found[j].Path
	})
}

// btWriteDelay is the pause required before the next BLE write.
func btWriteDelay(last time.Time, minGap time.Duration) time.Duration {
	if last.IsZero() || minGap <= 0 {
		return 0
	}
	if elapsed := time.Since(last); elapsed < minGap {
		return minGap - elapsed
	}
	return 0
}

// splitBTReport accepts either [reportID, prefix, payload...] or [prefix, payload...].
// The 64-byte payload is returned as-is. Callers must not require payload[0] to echo
// the opcode: 0x81 / 0x83 / 0x89 replies are raw data.
func splitBTReport(buf []byte, n int, wantPrefix byte) ([]byte, bool) {
	if n <= 0 || len(buf) < n {
		return nil, false
	}
	payloadStart := -1
	switch {
	case n >= 2 && buf[0] == btReportID && buf[1] == wantPrefix:
		payloadStart = 2
	case buf[0] == wantPrefix:
		payloadStart = 1
	default:
		return nil, false
	}
	out := make([]byte, 64)
	end := n
	if end > payloadStart+64 {
		end = payloadStart + 64
	}
	copy(out, buf[payloadStart:end])
	return out, true
}

// ---- Shared Feature Report Transports (USB Wired & 2.4G Dongle) ----

type featureConn interface {
	SendFeature(data []byte) error
	GetFeature() ([]byte, error)
	Close() error
}

type usbFeatureTransport struct {
	conn featureConn
	info DeviceInfo
}

func (t *usbFeatureTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *usbFeatureTransport) Close() error {
	if t.conn != nil {
		c := t.conn
		t.conn = nil
		return c.Close()
	}
	return nil
}

func (t *usbFeatureTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	if t.conn == nil {
		return nil, errors.New("设备连接已关闭")
	}
	if err := t.conn.SendFeature(cmd); err != nil {
		return nil, fmt.Errorf("USB Feature 写入失败: %w", err)
	}
	if !expectReply {
		return nil, nil
	}
	time.Sleep(15 * time.Millisecond)
	resp, err := t.conn.GetFeature()
	if err != nil {
		return nil, fmt.Errorf("USB Feature 读取失败: %w", err)
	}
	return resp, nil
}

func (t *usbFeatureTransport) ReadBattery() (int, uint16, error) {
	// Wired USB mice are always powered.
	return 100, 0, nil
}

type dongleStatus struct {
	canRead  bool
	battery  int
	isOnline bool
	canSend  bool
}

func parseDongleF7Response(resp []byte) (dongleStatus, error) {
	if len(resp) < 9 {
		return dongleStatus{}, errors.New("2.4G 接收器状态响应无效")
	}
	devMask := resp[6]
	if devMask != 2 && devMask != 3 {
		return dongleStatus{battery: -1}, nil
	}
	return dongleStatus{
		canRead:  resp[0] == 1,
		battery:  int(resp[2]),
		isOnline: resp[4] == 0,
		canSend:  resp[5] == 1,
	}, nil
}

type dongle24GTransport struct {
	conn           featureConn
	info           DeviceInfo
	channelCurrent string
	pollDelay      time.Duration // optional override for tests (0 -> default 10ms / 80ms)
}

func (t *dongle24GTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *dongle24GTransport) Close() error {
	if t.conn != nil {
		c := t.conn
		t.conn = nil
		return c.Close()
	}
	return nil
}

func (t *dongle24GTransport) stepSleep(d time.Duration) {
	if t.pollDelay > 0 {
		time.Sleep(t.pollDelay)
		return
	}
	if t.pollDelay < 0 {
		return
	}
	time.Sleep(d)
}

func (t *dongle24GTransport) checkF7() (dongleStatus, error) {
	t.stepSleep(10 * time.Millisecond)
	if err := t.conn.SendFeature([]byte{0xF7}); err != nil {
		return dongleStatus{}, err
	}
	t.stepSleep(10 * time.Millisecond)
	resp, err := t.conn.GetFeature()
	if err != nil {
		return dongleStatus{}, err
	}
	return parseDongleF7Response(resp)
}

func (t *dongle24GTransport) selectMouseChannel() error {
	if t.channelCurrent == "mouse" {
		return nil
	}
	t.stepSleep(10 * time.Millisecond)
	if err := t.conn.SendFeature([]byte{0xF6, 5}); err != nil {
		return err
	}
	t.channelCurrent = "mouse"
	return nil
}

func (t *dongle24GTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	if t.conn == nil {
		return nil, errors.New("设备连接已关闭")
	}
	// Wait until dongle reports mouse channel can send
	canSend := false
	for i := 0; i < 6; i++ {
		st, err := t.checkF7()
		if err != nil {
			return nil, fmt.Errorf("2.4G 状态查询失败: %w", err)
		}
		if !st.isOnline {
			return nil, errors.New("2.4G 鼠标当前处于休眠或离线状态，请移动鼠标唤醒")
		}
		if st.canSend {
			canSend = true
			break
		}
		t.stepSleep(80 * time.Millisecond)
	}
	if !canSend {
		return nil, errors.New("2.4G 接收器忙，请重试")
	}
	if err := t.selectMouseChannel(); err != nil {
		return nil, fmt.Errorf("2.4G 切换鼠标通道失败: %w", err)
	}
	t.stepSleep(10 * time.Millisecond)
	if err := t.conn.SendFeature(cmd); err != nil {
		return nil, fmt.Errorf("2.4G 发送命令失败: %w", err)
	}
	if !expectReply {
		return nil, nil
	}

	var zero8 [8]byte
	for i := 0; i < 6; i++ {
		t.stepSleep(80 * time.Millisecond)
		st, err := t.checkF7()
		if err != nil {
			return nil, err
		}
		if st.canRead {
			t.stepSleep(10 * time.Millisecond)
			if err := t.conn.SendFeature([]byte{0xFC}); err != nil {
				return nil, fmt.Errorf("2.4G 请求读取响应失败: %w", err)
			}
			t.stepSleep(10 * time.Millisecond)
			resp, err := t.conn.GetFeature()
			if err != nil {
				return nil, err
			}
			if len(resp) >= 8 && !bytes.Equal(resp[:8], zero8[:]) {
				return resp, nil
			}
		}
	}
	return nil, errors.New("2.4G 读取鼠标响应超时")
}

func (t *dongle24GTransport) ReadBattery() (int, uint16, error) {
	if t.conn == nil {
		return -1, 0, errors.New("设备连接已关闭")
	}
	st, err := t.checkF7()
	if err != nil {
		return -1, 0, err
	}
	if !st.isOnline {
		return -1, 0, nil
	}
	return st.battery, 0, nil
}
