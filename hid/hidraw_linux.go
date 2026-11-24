//go:build linux

package hid

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	hidIOCSFeature65 = 0xC0414806 // _IOWR('H', 0x06, 65)
	hidIOCGFeature65 = 0xC0414807 // _IOWR('H', 0x07, 65)

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

// Discover scans /sys/class/hidraw/* for supported Valkyrie / RongYuan mouse vendor config interfaces.
// Returned list is sorted by priority: USB wired > 2.4G dongle > Bluetooth.
func Discover() ([]DeviceInfo, error) {
	entries, err := os.ReadDir("/sys/class/hidraw")
	if err != nil {
		return nil, fmt.Errorf("读取 /sys/class/hidraw 失败: %w", err)
	}
	var found []DeviceInfo
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "hidraw") {
			continue
		}
		devDir := filepath.Join("/sys/class/hidraw", name, "device")
		ueventBytes, err := os.ReadFile(filepath.Join(devDir, "uevent"))
		if err != nil {
			continue
		}
		bus, vid, pid, hidName := parseUevent(string(ueventBytes))
		if !isSupportedVIDPID(vid, pid, hidName) {
			continue
		}
		descBytes, err := os.ReadFile(filepath.Join(devDir, "report_descriptor"))
		if err != nil {
			continue
		}
		meta := ParseReportDescriptor(descBytes)

		var mode ConnectionMode
		switch {
		case bus == 0x05 && meta.HasVendorBT: // BUS_BLUETOOTH
			mode = ModeBT
		case bus == 0x03 && meta.HasVendorUSB: // BUS_USB
			if pid == PIDVKM524G || strings.Contains(strings.ToLower(hidName), "2.4g") || strings.Contains(strings.ToLower(hidName), "dongle") || strings.Contains(strings.ToLower(hidName), "receiver") {
				mode = Mode24G
			} else {
				mode = ModeUSB
			}
		default:
			continue
		}

		found = append(found, DeviceInfo{
			Path:      filepath.Join("/dev", name),
			VID:       vid,
			PID:       pid,
			Name:      hidName,
			Mode:      mode,
			UsagePage: meta.VendorPage,
			ReportID:  meta.VendorReport,
		})
	}

	priority := map[ConnectionMode]int{ModeUSB: 0, Mode24G: 1, ModeBT: 2}
	sort.Slice(found, func(i, j int) bool {
		if priority[found[i].Mode] != priority[found[j].Mode] {
			return priority[found[i].Mode] < priority[found[j].Mode]
		}
		return found[i].Path < found[j].Path
	})
	return found, nil
}

func isSupportedVIDPID(vid, pid uint16, name string) bool {
	if vid == VIDValkyrie {
		return true
	}
	if vid == VIDRongYuan {
		low := strings.ToLower(name)
		if strings.Contains(low, "vk") || strings.Contains(low, "valkyrie") || strings.Contains(low, "mouse") || pid == 0xA321 || pid == 0xA322 || pid == 0xA323 {
			return true
		}
	}
	return false
}

func parseUevent(content string) (bus uint16, vid uint16, pid uint16, name string) {
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "HID_ID":
			parts := strings.Split(v, ":")
			if len(parts) == 3 {
				if b, err := strconv.ParseUint(parts[0], 16, 16); err == nil {
					bus = uint16(b)
				}
				if vv, err := strconv.ParseUint(parts[1], 16, 16); err == nil {
					vid = uint16(vv)
				}
				if pp, err := strconv.ParseUint(parts[2], 16, 16); err == nil {
					pid = uint16(pp)
				}
			}
		case "HID_NAME":
			name = v
		}
	}
	return
}

// OpenTransport opens the appropriate hidraw transport for the given discovered DeviceInfo.
func OpenTransport(info DeviceInfo) (Transport, error) {
	flags := syscall.O_RDWR | syscall.O_CLOEXEC
	if info.Mode == ModeBT {
		flags |= syscall.O_NONBLOCK
	}
	fd, err := syscall.Open(info.Path, flags, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) {
			return nil, fmt.Errorf("无权限访问 %s，请检查 udev 规则 (/etc/udev/rules.d/99-valkyrie-hid.rules): %w", info.Path, err)
		}
		return nil, fmt.Errorf("打开设备 %s 失败: %w", info.Path, err)
	}
	switch info.Mode {
	case ModeBT:
		return &btTransport{fd: fd, info: info}, nil
	case Mode24G:
		return &dongle24GTransport{fd: fd, info: info}, nil
	default:
		return &usbTransport{fd: fd, info: info}, nil
	}
}

// ---- Linux ppoll helper (works identically on arm64 & amd64 without CGO) ----

type pollFD struct {
	Fd      int32
	Events  int16
	Revents int16
}

const (
	pollIn  int16 = 0x0001
	pollErr int16 = 0x0008
	pollHup int16 = 0x0010
)

func waitReadable(fd int, timeout time.Duration) (bool, error) {
	pfd := pollFD{Fd: int32(fd), Events: pollIn}
	ts := syscall.NsecToTimespec(timeout.Nanoseconds())
	for {
		n, _, errno := syscall.Syscall6(
			syscall.SYS_PPOLL,
			uintptr(unsafe.Pointer(&pfd)),
			1,
			uintptr(unsafe.Pointer(&ts)),
			0,
			0,
			0,
		)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return false, errno
		}
		if n == 0 {
			return false, nil
		}
		if pfd.Revents&(pollErr|pollHup) != 0 {
			return false, errors.New("HID 设备已断开")
		}
		return pfd.Revents&pollIn != 0, nil
	}
}

// ---- Bluetooth (BLE HID over GATT) Transport ----

type btTransport struct {
	fd        int
	info      DeviceInfo
	lastWrite time.Time
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

func (t *btTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *btTransport) Close() error {
	if t.fd >= 0 {
		fd := t.fd
		t.fd = -1
		return syscall.Close(fd)
	}
	return nil
}

func (t *btTransport) drain() {
	var buf [128]byte
	for i := 0; i < 16; i++ {
		n, err := syscall.Read(t.fd, buf[:])
		if n <= 0 || err != nil {
			break
		}
	}
}

func (t *btTransport) roundTrip(prefix byte, payload []byte, expectReply bool) ([]byte, error) {
	if t.fd < 0 {
		return nil, errors.New("设备连接已关闭")
	}
	var pkt [btPacketLen]byte
	pkt[0] = btReportID
	pkt[1] = prefix
	copy(pkt[2:], payload)

	attempts := 3
	if !expectReply {
		attempts = 2
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if wait := btWriteDelay(t.lastWrite, btMinWriteGap); wait > 0 {
			time.Sleep(wait)
		}
		t.drain()
		n, err := syscall.Write(t.fd, pkt[:])
		if err != nil {
			lastErr = fmt.Errorf("蓝牙写入失败: %w", err)
			time.Sleep(40 * time.Millisecond)
			continue
		}
		if n != len(pkt) {
			lastErr = fmt.Errorf("蓝牙写入字节不完整: %d/%d", n, len(pkt))
			time.Sleep(40 * time.Millisecond)
			continue
		}
		t.lastWrite = time.Now()
		if !expectReply {
			return nil, nil
		}

		deadline := time.Now().Add(btReadWait)
		var rbuf [128]byte
		for time.Now().Before(deadline) {
			rem := time.Until(deadline)
			if rem <= 0 {
				break
			}
			ready, perr := waitReadable(t.fd, rem)
			if perr != nil {
				return nil, perr
			}
			if !ready {
				break
			}
			rn, rerr := syscall.Read(t.fd, rbuf[:])
			if rerr != nil {
				if errors.Is(rerr, syscall.EAGAIN) || errors.Is(rerr, syscall.EWOULDBLOCK) {
					continue
				}
				return nil, fmt.Errorf("蓝牙读取失败: %w", rerr)
			}
			if out, ok := splitBTReport(rbuf[:], rn, prefix); ok {
				return out, nil
			}
		}
		lastErr = errors.New("蓝牙通信超时，鼠标可能处于休眠状态，请轻动鼠标后重试")
		time.Sleep(60 * time.Millisecond)
	}
	return nil, lastErr
}

func (t *btTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	return t.roundTrip(btCmdPrefix, cmd, expectReply)
}

func (t *btTransport) ReadBattery() (int, uint16, error) {
	resp, err := t.roundTrip(btStatPrefix, nil, true)
	if err != nil {
		return -1, 0, err
	}
	if len(resp) < 3 {
		return -1, 0, errors.New("蓝牙状态响应过短")
	}
	batt := int(resp[0])
	if batt > 100 {
		batt = 100
	}
	fw := binary.LittleEndian.Uint16(resp[1:3])
	return batt, fw, nil
}

// ---- USB Wired Feature Report Transport ----

type usbTransport struct {
	fd   int
	info DeviceInfo
}

func (t *usbTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *usbTransport) Close() error {
	if t.fd >= 0 {
		fd := t.fd
		t.fd = -1
		return syscall.Close(fd)
	}
	return nil
}

func ioctlFeature(fd int, req uintptr, buf *[65]byte) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return errno
	}
	return nil
}

func (t *usbTransport) sendFeature(data []byte) error {
	var buf [65]byte
	buf[0] = t.info.ReportID
	copy(buf[1:], data)
	return ioctlFeature(t.fd, hidIOCSFeature65, &buf)
}

func (t *usbTransport) getFeature() ([]byte, error) {
	var buf [65]byte
	buf[0] = t.info.ReportID
	if err := ioctlFeature(t.fd, hidIOCGFeature65, &buf); err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	copy(out, buf[1:])
	return out, nil
}

func (t *usbTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	if t.fd < 0 {
		return nil, errors.New("设备连接已关闭")
	}
	if err := t.sendFeature(cmd); err != nil {
		return nil, fmt.Errorf("USB Feature 写入失败: %w", err)
	}
	if !expectReply {
		return nil, nil
	}
	time.Sleep(15 * time.Millisecond)
	resp, err := t.getFeature()
	if err != nil {
		return nil, fmt.Errorf("USB Feature 读取失败: %w", err)
	}
	return resp, nil
}

func (t *usbTransport) ReadBattery() (int, uint16, error) {
	// Wired USB mice are always powered; query command 0x8A if supported, else return 100.
	return 100, 0, nil
}

// ---- 2.4G Dongle Feature Report Transport (0xF7 / 0xF6 / 0xFC) ----

type dongle24GTransport struct {
	fd             int
	info           DeviceInfo
	channelCurrent string
}

func (t *dongle24GTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *dongle24GTransport) Close() error {
	if t.fd >= 0 {
		fd := t.fd
		t.fd = -1
		return syscall.Close(fd)
	}
	return nil
}

func (t *dongle24GTransport) sendFeature(data []byte) error {
	var buf [65]byte
	buf[0] = t.info.ReportID
	copy(buf[1:], data)
	return ioctlFeature(t.fd, hidIOCSFeature65, &buf)
}

func (t *dongle24GTransport) getFeature() ([]byte, error) {
	var buf [65]byte
	buf[0] = t.info.ReportID
	if err := ioctlFeature(t.fd, hidIOCGFeature65, &buf); err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	copy(out, buf[1:])
	return out, nil
}

type dongleStatus struct {
	canRead  bool
	battery  int
	isOnline bool
	canSend  bool
}

func (t *dongle24GTransport) checkF7() (dongleStatus, error) {
	time.Sleep(10 * time.Millisecond)
	if err := t.sendFeature([]byte{0xF7}); err != nil {
		return dongleStatus{}, err
	}
	time.Sleep(10 * time.Millisecond)
	resp, err := t.getFeature()
	if err != nil {
		return dongleStatus{}, err
	}
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

func (t *dongle24GTransport) selectMouseChannel() error {
	if t.channelCurrent == "mouse" {
		return nil
	}
	time.Sleep(10 * time.Millisecond)
	if err := t.sendFeature([]byte{0xF6, 5}); err != nil {
		return err
	}
	t.channelCurrent = "mouse"
	return nil
}

func (t *dongle24GTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	if t.fd < 0 {
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
		time.Sleep(80 * time.Millisecond)
	}
	if !canSend {
		return nil, errors.New("2.4G 接收器忙，请重试")
	}
	if err := t.selectMouseChannel(); err != nil {
		return nil, fmt.Errorf("2.4G 切换鼠标通道失败: %w", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := t.sendFeature(cmd); err != nil {
		return nil, fmt.Errorf("2.4G 发送命令失败: %w", err)
	}
	if !expectReply {
		return nil, nil
	}

	for i := 0; i < 6; i++ {
		time.Sleep(80 * time.Millisecond)
		st, err := t.checkF7()
		if err != nil {
			return nil, err
		}
		if st.canRead {
			time.Sleep(10 * time.Millisecond)
			if err := t.sendFeature([]byte{0xFC}); err != nil {
				return nil, fmt.Errorf("2.4G 请求读取响应失败: %w", err)
			}
			time.Sleep(10 * time.Millisecond)
			resp, err := t.getFeature()
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(resp[:8], make([]byte, 8)) {
				return resp, nil
			}
		}
	}
	return nil, errors.New("2.4G 读取鼠标响应超时")
}

func (t *dongle24GTransport) ReadBattery() (int, uint16, error) {
	st, err := t.checkF7()
	if err != nil {
		return -1, 0, err
	}
	if !st.isOnline {
		return -1, 0, nil
	}
	return st.battery, 0, nil
}
