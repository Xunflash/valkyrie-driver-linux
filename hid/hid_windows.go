//go:build windows

package hid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	hidDLL      = syscall.NewLazyDLL("hid.dll")
	setupapiDLL = syscall.NewLazyDLL("setupapi.dll")
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	procHidDGetHidGuid       = hidDLL.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes    = hidDLL.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData = hidDLL.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsed    = hidDLL.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps          = hidDLL.NewProc("HidP_GetCaps")
	procHidDGetProductString = hidDLL.NewProc("HidD_GetProductString")
	procHidDSetFeature       = hidDLL.NewProc("HidD_SetFeature")
	procHidDGetFeature       = hidDLL.NewProc("HidD_GetFeature")
	procHidDFlushQueue       = hidDLL.NewProc("HidD_FlushQueue")

	procSetupDiGetClassDevsW             = setupapiDLL.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = setupapiDLL.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = setupapiDLL.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = setupapiDLL.NewProc("SetupDiDestroyDeviceInfoList")

	procCreateEventW        = kernel32DLL.NewProc("CreateEventW")
	procResetEvent          = kernel32DLL.NewProc("ResetEvent")
	procGetOverlappedResult = kernel32DLL.NewProc("GetOverlappedResult")
	procCancelIoEx          = kernel32DLL.NewProc("CancelIoEx")
)

const (
	digcfPresent         uint32 = 0x00000002
	digcfDeviceInterface uint32 = 0x00000010
	hidpStatusSuccess    uint32 = 0x00110000
	errNoMoreItems              = syscall.Errno(259)
)

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type spDeviceInterfaceData struct {
	cbSize             uint32
	interfaceClassGuid winGUID
	flags              uint32
	reserved           uintptr
}

type hiddAttributes struct {
	size          uint32
	vendorID      uint16
	productID     uint16
	versionNumber uint16
}

type hidpCaps struct {
	usage                     uint16
	usagePage                 uint16
	inputReportByteLength     uint16
	outputReportByteLength    uint16
	featureReportByteLength   uint16
	reserved                  [17]uint16
	numberLinkCollectionNodes uint16
	numberInputButtonCaps     uint16
	numberInputValueCaps      uint16
	numberInputDataIndices    uint16
	numberOutputButtonCaps    uint16
	numberOutputValueCaps     uint16
	numberOutputDataIndices   uint16
	numberFeatureButtonCaps   uint16
	numberFeatureValueCaps    uint16
	numberFeatureDataIndices  uint16
}

// detailDataHeaderSize returns sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA_W):
// 8 bytes on 64-bit Windows (4B cbSize + 2B WCHAR + 2B pad), 6 bytes on 32-bit Windows.
func detailDataHeaderSize() uint32 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return 8
	}
	return 6
}

func decodeUTF16Bytes(b []byte) string {
	n := len(b) / 2
	if n == 0 {
		return ""
	}
	u16 := make([]uint16, 0, n)
	for i := 0; i < n; i++ {
		v := binary.LittleEndian.Uint16(b[i*2 : i*2+2])
		if v == 0 {
			break
		}
		u16 = append(u16, v)
	}
	return string(utf16.Decode(u16))
}

func queryCaps(h syscall.Handle) (hidpCaps, error) {
	var ppd uintptr
	r1, _, err := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&ppd)))
	if byte(r1) == 0 || ppd == 0 {
		return hidpCaps{}, fmt.Errorf("HidD_GetPreparsedData failed: %w", err)
	}
	defer procHidDFreePreparsed.Call(ppd)

	var caps hidpCaps
	st, _, _ := procHidPGetCaps.Call(ppd, uintptr(unsafe.Pointer(&caps)))
	if uint32(st) != hidpStatusSuccess {
		return hidpCaps{}, fmt.Errorf("HidP_GetCaps failed: 0x%08x", uint32(st))
	}
	return caps, nil
}

func queryProductString(h syscall.Handle) string {
	var buf [256]byte
	r1, _, _ := procHidDGetProductString.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if byte(r1) == 0 {
		return ""
	}
	return strings.TrimSpace(decodeUTF16Bytes(buf[:]))
}

// Discover enumerates Windows HID device interfaces for supported Valkyrie / RongYuan mice.
// Returned list is sorted by priority: USB wired > 2.4G dongle > Bluetooth.
func Discover() ([]DeviceInfo, error) {
	var hidGUID winGUID
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&hidGUID)))

	devInfo, _, err := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&hidGUID)),
		0,
		0,
		uintptr(digcfPresent|digcfDeviceInterface),
	)
	if devInfo == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("枚举 Windows HID 设备失败: %w", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(devInfo)

	var found []DeviceInfo
	for idx := uint32(0); ; idx++ {
		var did spDeviceInterfaceData
		did.cbSize = uint32(unsafe.Sizeof(did))
		r1, _, enumErr := procSetupDiEnumDeviceInterfaces.Call(
			devInfo,
			0,
			uintptr(unsafe.Pointer(&hidGUID)),
			uintptr(idx),
			uintptr(unsafe.Pointer(&did)),
		)
		if uint32(r1) == 0 {
			if errors.Is(enumErr, errNoMoreItems) {
				break
			}
			break
		}

		var reqSize uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(
			devInfo,
			uintptr(unsafe.Pointer(&did)),
			0,
			0,
			uintptr(unsafe.Pointer(&reqSize)),
			0,
		)
		if reqSize <= 4 || reqSize > 16384 {
			continue
		}

		detailBuf := make([]byte, reqSize)
		binary.LittleEndian.PutUint32(detailBuf[0:4], detailDataHeaderSize())
		ok, _, _ := procSetupDiGetDeviceInterfaceDetailW.Call(
			devInfo,
			uintptr(unsafe.Pointer(&did)),
			uintptr(unsafe.Pointer(&detailBuf[0])),
			uintptr(reqSize),
			0,
			0,
		)
		if uint32(ok) == 0 {
			continue
		}

		devPath := decodeUTF16Bytes(detailBuf[4:])
		if devPath == "" {
			continue
		}

		pathPtr, perr := syscall.UTF16PtrFromString(devPath)
		if perr != nil {
			continue
		}
		// DesiredAccess=0 allows inspecting HID attributes & caps without conflicting with exclusive devices.
		h, cerr := syscall.CreateFile(
			pathPtr,
			0,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
			nil,
			syscall.OPEN_EXISTING,
			0,
			0,
		)
		if cerr != nil || h == syscall.InvalidHandle {
			continue
		}

		var attrs hiddAttributes
		attrs.size = uint32(unsafe.Sizeof(attrs))
		ar, _, _ := procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs)))
		if byte(ar) == 0 || (attrs.vendorID != VIDValkyrie && attrs.vendorID != VIDRongYuan) {
			_ = syscall.CloseHandle(h)
			continue
		}

		prodName := queryProductString(h)
		if !isSupportedVIDPID(attrs.vendorID, attrs.productID, prodName) {
			_ = syscall.CloseHandle(h)
			continue
		}

		caps, capErr := queryCaps(h)
		_ = syscall.CloseHandle(h)
		if capErr != nil {
			continue
		}

		var mode ConnectionMode
		var reportID byte
		switch {
		case caps.usagePage == 0xFF55 && (caps.outputReportByteLength >= 65 || caps.inputReportByteLength >= 65):
			mode = ModeBT
			reportID = btReportID
		case caps.usagePage == 0xFFFF && (caps.featureReportByteLength >= 65 || caps.outputReportByteLength >= 65):
			if is24GDongle(attrs.productID, prodName) {
				mode = Mode24G
			} else {
				mode = ModeUSB
			}
			reportID = 0
		default:
			continue
		}

		if prodName == "" {
			prodName = fmt.Sprintf("Valkyrie (%04X:%04X)", attrs.vendorID, attrs.productID)
		}

		found = append(found, DeviceInfo{
			Path:      devPath,
			VID:       attrs.vendorID,
			PID:       attrs.productID,
			Name:      prodName,
			Mode:      mode,
			UsagePage: caps.usagePage,
			ReportID:  reportID,
		})
	}

	sortDevicesByPriority(found)
	return found, nil
}

// OpenTransport opens the appropriate Windows HID transport for the given discovered DeviceInfo.
func OpenTransport(info DeviceInfo) (Transport, error) {
	pathPtr, err := syscall.UTF16PtrFromString(info.Path)
	if err != nil {
		return nil, fmt.Errorf("无效 HID 设备路径: %w", err)
	}

	var attrs uint32
	if info.Mode == ModeBT {
		attrs = syscall.FILE_FLAG_OVERLAPPED
	}
	h, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		attrs,
		0,
	)
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("无法访问 Windows HID 设备（请先退出官方驱动或其他占用该鼠标的工具后重试）: %w", err)
		}
		return nil, fmt.Errorf("打开 Windows HID 设备失败: %w", err)
	}

	caps, _ := queryCaps(h)

	switch info.Mode {
	case ModeBT:
		readEv, err := createManualResetEvent()
		if err != nil {
			_ = syscall.CloseHandle(h)
			return nil, err
		}
		writeEv, err := createManualResetEvent()
		if err != nil {
			_ = syscall.CloseHandle(readEv)
			_ = syscall.CloseHandle(h)
			return nil, err
		}
		outLen := int(caps.outputReportByteLength)
		if outLen < btPacketLen {
			outLen = btPacketLen
		}
		inLen := int(caps.inputReportByteLength)
		if inLen < btPacketLen {
			inLen = btPacketLen
		}
		return &winBTTransport{
			handle:   h,
			info:     info,
			readEv:   readEv,
			writeEv:  writeEv,
			writeBuf: make([]byte, outLen),
			readBuf:  make([]byte, inLen),
		}, nil

	case Mode24G:
		featLen := int(caps.featureReportByteLength)
		if featLen < 65 {
			featLen = 65
		}
		return &dongle24GTransport{
			conn: &winFeatureConn{handle: h, reportID: info.ReportID, featLen: featLen},
			info: info,
		}, nil

	default:
		featLen := int(caps.featureReportByteLength)
		if featLen < 65 {
			featLen = 65
		}
		return &usbFeatureTransport{
			conn: &winFeatureConn{handle: h, reportID: info.ReportID, featLen: featLen},
			info: info,
		}, nil
	}
}

func createManualResetEvent() (syscall.Handle, error) {
	r1, _, err := procCreateEventW.Call(0, 1, 0, 0)
	if r1 == 0 {
		return 0, fmt.Errorf("CreateEventW failed: %w", err)
	}
	return syscall.Handle(r1), nil
}

// ---- Windows Bluetooth (BLE HID over GATT) Transport ----

type winBTTransport struct {
	handle    syscall.Handle
	info      DeviceInfo
	readEv    syscall.Handle
	writeEv   syscall.Handle
	readOv    syscall.Overlapped
	writeOv   syscall.Overlapped
	writeBuf  []byte
	readBuf   []byte
	lastWrite time.Time
}

func (t *winBTTransport) DeviceInfo() DeviceInfo { return t.info }

func (t *winBTTransport) Close() error {
	var firstErr error
	if t.handle != 0 && t.handle != syscall.InvalidHandle {
		if err := syscall.CloseHandle(t.handle); err != nil {
			firstErr = err
		}
		t.handle = syscall.InvalidHandle
	}
	if t.readEv != 0 {
		_ = syscall.CloseHandle(t.readEv)
		t.readEv = 0
	}
	if t.writeEv != 0 {
		_ = syscall.CloseHandle(t.writeEv)
		t.writeEv = 0
	}
	return firstErr
}

func (t *winBTTransport) writeOverlapped(timeout time.Duration) (int, error) {
	procResetEvent.Call(uintptr(t.writeEv))
	t.writeOv = syscall.Overlapped{HEvent: t.writeEv}

	var written uint32
	err := syscall.WriteFile(t.handle, t.writeBuf, &written, &t.writeOv)
	if err == nil {
		return int(written), nil
	}
	if !errors.Is(err, syscall.ERROR_IO_PENDING) {
		return 0, err
	}

	ms := uint32(timeout.Milliseconds())
	if ms == 0 {
		ms = 1
	}
	waitRes, werr := syscall.WaitForSingleObject(t.writeEv, ms)
	if werr != nil {
		procCancelIoEx.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.writeOv)))
		procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.writeOv)), uintptr(unsafe.Pointer(&written)), 1)
		return 0, werr
	}
	if waitRes == syscall.WAIT_TIMEOUT {
		procCancelIoEx.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.writeOv)))
		procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.writeOv)), uintptr(unsafe.Pointer(&written)), 1)
		return 0, errors.New("write timeout")
	}
	ok, _, gerr := procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.writeOv)), uintptr(unsafe.Pointer(&written)), 0)
	if uint32(ok) == 0 {
		return 0, gerr
	}
	return int(written), nil
}

func (t *winBTTransport) readOverlapped(timeout time.Duration) (int, bool, error) {
	procResetEvent.Call(uintptr(t.readEv))
	t.readOv = syscall.Overlapped{HEvent: t.readEv}
	clear(t.readBuf)

	var n uint32
	err := syscall.ReadFile(t.handle, t.readBuf, &n, &t.readOv)
	if err == nil {
		return int(n), true, nil
	}
	if !errors.Is(err, syscall.ERROR_IO_PENDING) {
		return 0, false, err
	}

	ms := uint32(timeout.Milliseconds())
	if ms == 0 {
		ms = 1
	}
	waitRes, werr := syscall.WaitForSingleObject(t.readEv, ms)
	if werr != nil {
		procCancelIoEx.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.readOv)))
		procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.readOv)), uintptr(unsafe.Pointer(&n)), 1)
		return 0, false, werr
	}
	if waitRes == syscall.WAIT_TIMEOUT {
		procCancelIoEx.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.readOv)))
		procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.readOv)), uintptr(unsafe.Pointer(&n)), 1)
		return 0, false, nil
	}
	ok, _, gerr := procGetOverlappedResult.Call(uintptr(t.handle), uintptr(unsafe.Pointer(&t.readOv)), uintptr(unsafe.Pointer(&n)), 0)
	if uint32(ok) == 0 {
		return 0, false, gerr
	}
	return int(n), true, nil
}

func (t *winBTTransport) roundTrip(prefix byte, payload []byte, expectReply bool) ([]byte, error) {
	if t.handle == 0 || t.handle == syscall.InvalidHandle {
		return nil, errors.New("设备连接已关闭")
	}

	clear(t.writeBuf)
	t.writeBuf[0] = btReportID
	t.writeBuf[1] = prefix
	copy(t.writeBuf[2:], payload)

	attempts := 3
	if !expectReply {
		attempts = 2
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if wait := btWriteDelay(t.lastWrite, btMinWriteGap); wait > 0 {
			time.Sleep(wait)
		}
		procHidDFlushQueue.Call(uintptr(t.handle))

		n, err := t.writeOverlapped(500 * time.Millisecond)
		if err != nil {
			lastErr = fmt.Errorf("蓝牙写入失败: %w", err)
			time.Sleep(40 * time.Millisecond)
			continue
		}
		if n < btPacketLen {
			lastErr = fmt.Errorf("蓝牙写入字节不完整: %d/%d", n, len(t.writeBuf))
			time.Sleep(40 * time.Millisecond)
			continue
		}
		t.lastWrite = time.Now()
		if !expectReply {
			return nil, nil
		}

		deadline := time.Now().Add(btReadWait)
		for time.Now().Before(deadline) {
			rem := time.Until(deadline)
			if rem <= 0 {
				break
			}
			rn, got, rerr := t.readOverlapped(rem)
			if rerr != nil {
				return nil, fmt.Errorf("蓝牙读取失败: %w", rerr)
			}
			if !got {
				break
			}
			if out, ok := splitBTReport(t.readBuf, rn, prefix); ok {
				return out, nil
			}
		}
		lastErr = errors.New("蓝牙通信超时，鼠标可能处于休眠状态，请轻动鼠标后重试")
		time.Sleep(60 * time.Millisecond)
	}
	return nil, lastErr
}

func (t *winBTTransport) Exchange(cmd []byte, expectReply bool) ([]byte, error) {
	return t.roundTrip(btCmdPrefix, cmd, expectReply)
}

func (t *winBTTransport) ReadBattery() (int, uint16, error) {
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

// ---- Windows Feature Report backend (USB Wired & 2.4G Dongle) ----

type winFeatureConn struct {
	handle   syscall.Handle
	reportID byte
	featLen  int
}

func (c *winFeatureConn) SendFeature(data []byte) error {
	if c.handle == 0 || c.handle == syscall.InvalidHandle {
		return errors.New("设备连接已关闭")
	}
	buf := make([]byte, c.featLen)
	buf[0] = c.reportID
	copy(buf[1:], data)
	r1, _, err := procHidDSetFeature.Call(
		uintptr(c.handle),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if byte(r1) == 0 {
		return fmt.Errorf("HidD_SetFeature failed: %w", err)
	}
	return nil
}

func (c *winFeatureConn) GetFeature() ([]byte, error) {
	if c.handle == 0 || c.handle == syscall.InvalidHandle {
		return nil, errors.New("设备连接已关闭")
	}
	buf := make([]byte, c.featLen)
	buf[0] = c.reportID
	r1, _, err := procHidDGetFeature.Call(
		uintptr(c.handle),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if byte(r1) == 0 {
		return nil, fmt.Errorf("HidD_GetFeature failed: %w", err)
	}
	out := make([]byte, 64)
	copy(out, buf[1:])
	return out, nil
}

func (c *winFeatureConn) Close() error {
	if c.handle != 0 && c.handle != syscall.InvalidHandle {
		h := c.handle
		c.handle = syscall.InvalidHandle
		return syscall.CloseHandle(h)
	}
	return nil
}
