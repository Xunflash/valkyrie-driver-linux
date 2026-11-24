//go:build linux

package hid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	hidIOCSFeature65 = 0xC0414806 // _IOWR('H', 0x06, 65)
	hidIOCGFeature65 = 0xC0414807 // _IOWR('H', 0x07, 65)
)

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
			if is24GDongle(pid, hidName) {
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

	sortDevicesByPriority(found)
	return found, nil
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
		return &dongle24GTransport{
			conn: &linuxFeatureConn{fd: fd, reportID: info.ReportID},
			info: info,
		}, nil
	default:
		return &usbFeatureTransport{
			conn: &linuxFeatureConn{fd: fd, reportID: info.ReportID},
			info: info,
		}, nil
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

// ---- Linux Feature Report ioctl backend ----

type linuxFeatureConn struct {
	fd       int
	reportID byte
}

func ioctlFeature(fd int, req uintptr, buf *[65]byte) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return errno
	}
	return nil
}

func (c *linuxFeatureConn) SendFeature(data []byte) error {
	if c.fd < 0 {
		return errors.New("设备连接已关闭")
	}
	var buf [65]byte
	buf[0] = c.reportID
	copy(buf[1:], data)
	return ioctlFeature(c.fd, hidIOCSFeature65, &buf)
}

func (c *linuxFeatureConn) GetFeature() ([]byte, error) {
	if c.fd < 0 {
		return nil, errors.New("设备连接已关闭")
	}
	var buf [65]byte
	buf[0] = c.reportID
	if err := ioctlFeature(c.fd, hidIOCGFeature65, &buf); err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	copy(out, buf[1:])
	return out, nil
}

func (c *linuxFeatureConn) Close() error {
	if c.fd >= 0 {
		fd := c.fd
		c.fd = -1
		return syscall.Close(fd)
	}
	return nil
}
