package hid

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CommonMSCH585 command codes (verified against Valkyrie VK M5 official driver & live hardware).
const (
	CmdGetKeyMatrix   byte = 0x81 // 129, raw 64B matrix (command byte is not echoed)
	CmdGetProfile     byte = 0x82 // 130
	CmdGetMacro       byte = 0x83 // 131, raw macro bytes (command byte is not echoed)
	CmdGetDebounce    byte = 0x84 // 132
	CmdGetScrollUp    byte = 0x85 // 133
	CmdGetSleep24G    byte = 0x86 // 134
	CmdGetSleepBT     byte = 0x87 // 135
	CmdGetReportRate  byte = 0x88 // 136
	CmdGetBattery     byte = 0x8A // 138
	CmdGetGun         byte = 0x89 // 137, raw recoil bytes (command byte is not echoed)
	CmdGetLowLatency  byte = 0x8C // 140
	CmdGetTrue8K      byte = 0x8D // 141
	CmdGetVersionID   byte = 0x8F // 143
	CmdGetDPI         byte = 0x90 // 144
	CmdGetSilentH     byte = 0x91 // 145
	CmdGetLineRepair  byte = 0x92 // 146
	CmdGetWaveRepair  byte = 0x93 // 147
	CmdGetMotionSync  byte = 0x94 // 148
	CmdGetFPS20000    byte = 0x95 // 149
	CmdGetSensorAngle byte = 0x98 // 152
	CmdGetAllParams0  byte = 0x9F // 159

	CmdSetKeySimple   byte = 0x00 // 0
	CmdSetAllKeys     byte = 0x01 // 1
	CmdSetProfile     byte = 0x02 // 2
	CmdSetMacro       byte = 0x03 // 3
	CmdSetDebounce    byte = 0x04 // 4
	CmdSetScrollUp    byte = 0x05 // 5
	CmdSetSleep24G    byte = 0x06 // 6
	CmdSetSleepBT     byte = 0x07 // 7
	CmdSetReportRate  byte = 0x08 // 8
	CmdSetLowLatency  byte = 0x0C // 12
	CmdSetTrue8K      byte = 0x0D // 13
	CmdSetReset       byte = 0x0E // 14
	CmdSetDPI         byte = 0x10 // 16
	CmdSetSilentH     byte = 0x11 // 17
	CmdSetLineRepair  byte = 0x12 // 18
	CmdSetWaveRepair  byte = 0x13 // 19
	CmdSetMotionSync  byte = 0x14 // 20
	CmdSetFPS20000    byte = 0x15 // 21
	CmdSetSensorAngle byte = 0x18 // 24
)

// SupportedReportRates lists all valid polling rates in Hz.
var SupportedReportRates = []int{125, 250, 500, 1000, 2000, 4000, 8000}

// EncodeBit7 builds a 64-byte CommonMSCH585 command frame with Bit7 checksum at byte[7].
func EncodeBit7(payload []byte) []byte {
	frame := make([]byte, 64)
	copy(frame, payload)
	var sum uint32
	for i := 0; i < 7; i++ {
		sum += uint32(frame[i])
	}
	frame[7] = byte(255 - (sum & 0xFF))
	return frame
}

// EncodeReportRate converts polling rate in Hz to its firmware wire code.
func EncodeReportRate(hz int) (byte, error) {
	switch hz {
	case 8000:
		return 129, nil
	case 4000:
		return 130, nil
	case 2000:
		return 132, nil
	case 1000:
		return 1, nil
	case 500:
		return 2, nil
	case 250:
		return 4, nil
	case 125:
		return 8, nil
	default:
		return 0, fmt.Errorf("不支持的回报率: %d Hz", hz)
	}
}

// DecodeReportRate converts a firmware wire code to polling rate in Hz.
func DecodeReportRate(code byte) int {
	switch code {
	case 129:
		return 8000
	case 130:
		return 4000
	case 132:
		return 2000
	case 1:
		return 1000
	case 2:
		return 500
	case 4:
		return 250
	case 8:
		return 125
	default:
		return 125
	}
}

// EncodeAngle converts degrees (-30..+30) to the unsigned wire byte (two's complement).
func EncodeAngle(deg int8) byte {
	return byte(deg)
}

// DecodeAngle converts the unsigned wire byte back to signed degrees (-30..+30).
func DecodeAngle(b byte) int8 {
	if b <= 90 {
		return int8(b)
	}
	return int8(int(b) - 256)
}

// Client speaks the RongYuan CommonMSCH585 protocol over a Transport.
type Client struct {
	mu          sync.Mutex
	transport   Transport
	vendorSleep time.Duration
}

// NewClient wraps a Transport with serialized protocol operations.
// Post-write settle time follows the official driver: Bluetooth 100 ms, USB 40 ms, 2.4G 100 ms.
func NewClient(t Transport) *Client {
	sleep := 100 * time.Millisecond
	if t != nil && t.DeviceInfo().Mode == ModeUSB {
		sleep = 40 * time.Millisecond
	}
	return &Client{
		transport:   t,
		vendorSleep: sleep,
	}
}

// SetVendorSleep overrides the post-write EEPROM settle delay (used by unit tests).
func (c *Client) SetVendorSleep(d time.Duration) {
	c.mu.Lock()
	c.vendorSleep = d
	c.mu.Unlock()
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transport == nil {
		return nil
	}
	err := c.transport.Close()
	c.transport = nil
	return err
}

func (c *Client) queryLocked(payload []byte) ([]byte, error) {
	if c.transport == nil {
		return nil, errors.New("设备未连接")
	}
	frame := EncodeBit7(payload)
	return c.transport.Exchange(frame, true)
}

func (c *Client) writeLocked(payload []byte, sleepOverride time.Duration) error {
	if c.transport == nil {
		return errors.New("设备未连接")
	}
	frame := EncodeBit7(payload)
	// Set_* commands never return a packet on BT, USB, or 2.4G.
	// Waiting here made every Bluetooth write burn 3×450 ms of timeouts.
	if _, err := c.transport.Exchange(frame, false); err != nil {
		return err
	}
	// SetVendorSleep(0) is the unit-test fast path and also skips the factory-reset override.
	if c.vendorSleep == 0 {
		return nil
	}
	wait := c.vendorSleep
	if sleepOverride > 0 {
		wait = sleepOverride
	}
	if wait > 0 {
		time.Sleep(wait)
	}
	return nil
}

// ReadVersionAndID reads device ID (uint32) and firmware version (uint16) via 0x8F.
func (c *Client) ReadVersionAndID() (uint32, uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readVersionAndIDLocked()
}

func (c *Client) readVersionAndIDLocked() (uint32, uint16, error) {
	resp, err := c.queryLocked([]byte{CmdGetVersionID})
	if err != nil {
		return 0, 0, err
	}
	if len(resp) < 7 || resp[0] != CmdGetVersionID {
		return 0, 0, fmt.Errorf("无效的版本响应头: 0x%02x", resp[0])
	}
	devID := binary.LittleEndian.Uint32(resp[1:5])
	fwVer := binary.LittleEndian.Uint16(resp[5:7])
	return devID, fwVer, nil
}

// ReadBattery queries the live battery percentage (0..100).
func (c *Client) ReadBattery() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transport == nil {
		return -1, errors.New("设备未连接")
	}
	batt, _, err := c.transport.ReadBattery()
	return batt, err
}

// ReadAllParams reads 0x9F AllParams0 plus angle/lowLatency/fps fallbacks.
func (c *Client) ReadAllParams() (MouseParams, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readAllParamsLocked()
}

func (c *Client) readAllParamsLocked() (MouseParams, error) {
	resp, err := c.queryLocked([]byte{CmdGetAllParams0})
	if err != nil {
		return MouseParams{}, err
	}
	if len(resp) < 42 {
		return MouseParams{}, errors.New("参数响应长度不足")
	}
	p := MouseParams{
		Profile:      resp[8],
		Debounce:     resp[9],
		ScrollUpTime: resp[10],
		Sleep24G:     binary.LittleEndian.Uint16(resp[11:13]),
		SleepBT:      binary.LittleEndian.Uint16(resp[13:15]),
		ReportRate:   DecodeReportRate(resp[15]),
		SilentHeight: resp[16],
		LineRepair:   resp[17] == 1,
		WaveRepair:   resp[18] == 1,
		MotionSync:   resp[19] == 1,
		FPS20000:     resp[28] == 1,
		AngleTune:    DecodeAngle(resp[39]),
		LowLatency:   resp[40] == 1,
		True8K:       resp[41] == 1,
	}
	for i := 0; i < 8; i++ {
		p.ListReportRate[i] = DecodeReportRate(resp[20+i])
	}
	return p, nil
}

// ReadDPI reads all DPI stages for the given profile (0..7) via 0x90.
func (c *Client) ReadDPI(profile uint8, stageCount int) (DPIConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readDPILocked(profile, stageCount)
}

func (c *Client) readDPILocked(profile uint8, stageCount int) (DPIConfig, error) {
	if profile > 7 {
		profile = 0
	}
	resp, err := c.queryLocked([]byte{CmdGetDPI, profile})
	if err != nil {
		return DPIConfig{}, err
	}
	if len(resp) < 64 {
		return DPIConfig{}, errors.New("DPI 响应长度不足")
	}
	count := int(resp[3])
	if count <= 0 || count > 8 {
		if stageCount > 0 && stageCount <= 8 {
			count = stageCount
		} else {
			count = 6
		}
	}
	cur := int(resp[2])
	if cur >= count {
		cur = 0
	}
	stages := make([]DPIStage, count)
	for i := 0; i < count; i++ {
		x := binary.LittleEndian.Uint16(resp[8+i*2 : 10+i*2])
		y := binary.LittleEndian.Uint16(resp[24+i*2 : 26+i*2])
		if y == 0 {
			y = x
		}
		stages[i] = DPIStage{
			X: x,
			Y: y,
			R: resp[40+i*3],
			G: resp[41+i*3],
			B: resp[42+i*3],
		}
	}
	return DPIConfig{
		Current: cur,
		Count:   count,
		Stages:  stages,
	}, nil
}

// ReadKeyMatrix reads the 16x4-byte button matrix for profile via 0x81.
func (c *Client) ReadKeyMatrix(profile uint8) ([16][4]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readKeyMatrixLocked(profile)
}

func (c *Client) readKeyMatrixLocked(profile uint8) ([16][4]byte, error) {
	var keys [16][4]byte
	resp, err := c.queryLocked([]byte{CmdGetKeyMatrix, profile, 1})
	if err != nil {
		return keys, err
	}
	if len(resp) < 64 {
		return keys, errors.New("按键矩阵响应长度不足")
	}
	for i := 0; i < 16; i++ {
		copy(keys[i][:], resp[i*4:i*4+4])
	}
	return keys, nil
}

// ReadAll reads the full mouse state snapshot (ID/FW, battery, params, DPI, keys).
func (c *Client) ReadAll() (*MouseState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transport == nil {
		return nil, errors.New("设备未连接")
	}
	info := c.transport.DeviceInfo()

	devID, fwVer, err := c.readVersionAndIDLocked()
	if err != nil {
		return nil, fmt.Errorf("读取设备版本/ID 失败: %w", err)
	}
	spec := LookupModel(devID, info.Name)

	batt, battFW, berr := c.transport.ReadBattery()
	if berr != nil {
		batt = -1
	}
	if fwVer == 0 && battFW != 0 {
		fwVer = battFW
	}

	params, err := c.readAllParamsLocked()
	if err != nil {
		return nil, fmt.Errorf("读取鼠标参数失败: %w", err)
	}
	// Query dedicated angle register if supported (0x98), because older firmware omits byte 39 in 0x9F.
	if angResp, aerr := c.queryLocked([]byte{CmdGetSensorAngle}); aerr == nil && len(angResp) >= 2 && angResp[0] == CmdGetSensorAngle {
		params.AngleTune = DecodeAngle(angResp[1])
	}

	dpi, err := c.readDPILocked(params.Profile, spec.DPIStages)
	if err != nil {
		return nil, fmt.Errorf("读取 DPI 设置失败: %w", err)
	}
	keys, err := c.readKeyMatrixLocked(params.Profile)
	if err != nil {
		return nil, fmt.Errorf("读取按键映射失败: %w", err)
	}

	return &MouseState{
		Info:        info,
		Spec:        spec,
		DevID:       devID,
		FirmwareVer: fwVer,
		Battery:     batt,
		Params:      params,
		DPI:         dpi,
		Keys:        keys,
	}, nil
}

// SetDPI writes DPI stages and the active index to a profile via 0x10.
// Count may be any value from 1 to 8. When Count is set and smaller than len(Stages)
// (for example 4 stages on a 6-stage buffer), byte 3 carries that count.
func (c *Client) SetDPI(profile uint8, cfg DPIConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := cfg.Count
	if count <= 0 {
		count = len(cfg.Stages)
	}
	if count < 1 || count > 8 {
		return fmt.Errorf("DPI 档位数量无效: %d", count)
	}
	if len(cfg.Stages) < count {
		return fmt.Errorf("DPI 档位数量不足: 需要 %d 档, 实际 %d", count, len(cfg.Stages))
	}
	if profile > 7 {
		profile = 0
	}
	cur := cfg.Current
	if cur < 0 || cur >= count {
		cur = 0
	}
	// Official firmware guard: reject writing if every sent stage has RGB == (0,0,0).
	allZeroRGB := true
	for _, st := range cfg.Stages[:count] {
		if st.R != 0 || st.G != 0 || st.B != 0 {
			allZeroRGB = false
			break
		}
	}
	payload := make([]byte, 64)
	payload[0] = CmdSetDPI
	payload[1] = profile
	payload[2] = byte(cur)
	payload[3] = byte(count)
	for i, st := range cfg.Stages[:count] {
		binary.LittleEndian.PutUint16(payload[8+i*2:10+i*2], st.X)
		binary.LittleEndian.PutUint16(payload[24+i*2:26+i*2], st.Y)
		r, g, b := st.R, st.G, st.B
		if allZeroRGB && i == 0 {
			b = 1 // ensure firmware never ignores the DPI write packet
		}
		payload[40+i*3] = r
		payload[41+i*3] = g
		payload[42+i*3] = b
	}
	if err := c.writeLocked(payload, 180*time.Millisecond); err != nil {
		return err
	}
	// On live hardware (vendorSleep > 0), verify the mouse actually committed the DPI packet
	// and retry once if BLE dropped it while waking from micro-sleep.
	if c.vendorSleep > 0 {
		want := cfg.Stages[cur]
		got, err := c.readDPILocked(profile, count)
		if err != nil || got.Current != cur || got.Count != count ||
			cur >= len(got.Stages) || got.Stages[cur].X != want.X || got.Stages[cur].Y != want.Y {
			if err := c.writeLocked(payload, 220*time.Millisecond); err != nil {
				return err
			}
			got2, err2 := c.readDPILocked(profile, count)
			if err2 != nil {
				return fmt.Errorf("写入 DPI 后校验读取失败: %w", err2)
			}
			if got2.Current != cur || cur >= len(got2.Stages) || got2.Stages[cur].X != want.X || got2.Stages[cur].Y != want.Y {
				return fmt.Errorf("鼠标未应用 DPI 设置（期望档位 %d=%d/%d，实际档位 %d=%d/%d），请轻移鼠标唤醒后重试",
					cur+1, want.X, want.Y, got2.Current+1, got2.Stages[got2.Current].X, got2.Stages[got2.Current].Y)
			}
		}
	}
	return nil
}

// SetAllKeys writes the full 16×4 matrix with command 0x01 (two packets, 56 bytes each).
func (c *Client) SetAllKeys(profile uint8, matrix [16][4]byte) error {
	if profile > 7 {
		profile = 0
	}
	var raw [64]byte
	for i := 0; i < 16; i++ {
		copy(raw[i*4:i*4+4], matrix[i][:])
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for pkt := 0; pkt < 2; pkt++ {
		payload := make([]byte, 64)
		payload[0] = CmdSetAllKeys
		payload[1] = profile
		payload[2] = byte(pkt)
		start := pkt * 56
		copy(payload[8:], raw[start:])
		if err := c.writeLocked(payload, 0); err != nil {
			return err
		}
	}
	return nil
}

// SetTrue8K writes the 2.4G true-8K switch via 0x0D.
func (c *Client) SetTrue8K(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetTrue8K, boolByte(on)}, 0)
}

// ReadTrue8K reads the 2.4G true-8K switch via 0x8D.
func (c *Client) ReadTrue8K() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, err := c.queryLocked([]byte{CmdGetTrue8K})
	if err != nil {
		return false, err
	}
	if len(resp) < 2 || resp[0] != CmdGetTrue8K {
		return false, errors.New("真 8K 响应无效")
	}
	return resp[1] == 1, nil
}

// ApplyPerformanceMode writes the official preset: report rate, then the 20K FPS flag.
func (c *Client) ApplyPerformanceMode(mode PerformanceMode) error {
	hz, fps, err := mode.Preset()
	if err != nil {
		return err
	}
	if err := c.SetReportRate(hz); err != nil {
		return err
	}
	return c.SetFPS20000(fps)
}

func (c *Client) SetReportRate(hz int) error {
	code, err := EncodeReportRate(hz)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetReportRate, code}, 0)
}

func (c *Client) SetDebounce(ms uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetDebounce, ms}, 0)
}

func (c *Client) SetScrollUpTime(ms uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetScrollUp, ms}, 0)
}

func (c *Client) SetSleep24G(sec uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetSleep24G, byte(sec & 0xFF), byte(sec >> 8)}, 0)
}

func (c *Client) SetSleepBT(sec uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetSleepBT, byte(sec & 0xFF), byte(sec >> 8)}, 0)
}

func (c *Client) SetSilentHeight(level uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetSilentH, level}, 0)
}

func (c *Client) SetLineRepair(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetLineRepair, boolByte(on)}, 0)
}

func (c *Client) SetWaveRepair(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetWaveRepair, boolByte(on)}, 0)
}

func (c *Client) SetMotionSync(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetMotionSync, boolByte(on)}, 0)
}

func (c *Client) SetFPS20000(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetFPS20000, boolByte(on)}, 0)
}

func (c *Client) SetLowLatency(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetLowLatency, boolByte(on)}, 0)
}

func (c *Client) SetSensorAngle(deg int8) error {
	if deg < -30 {
		deg = -30
	} else if deg > 30 {
		deg = 30
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetSensorAngle, EncodeAngle(deg)}, 0)
}

func (c *Client) SetCurrentProfile(profile uint8) error {
	if profile > 7 {
		profile = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetProfile, profile}, 0)
}

func (c *Client) SetKeySimple(btnIdx uint8, action [4]byte) error {
	if btnIdx >= 16 {
		return fmt.Errorf("按键编号超出范围: %d", btnIdx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	payload := make([]byte, 64)
	payload[0] = CmdSetKeySimple
	payload[1] = btnIdx
	copy(payload[8:12], action[:])
	return c.writeLocked(payload, 0)
}

func (c *Client) FactoryReset() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked([]byte{CmdSetReset}, 350*time.Millisecond)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// CheckLatestFirmware queries RongYuan's official API for the latest firmware version of devID.
func CheckLatestFirmware(ctx context.Context, devID uint32, currentFW uint16) FirmwareCheckResult {
	curStr := fmt.Sprintf("v%04x", currentFW)
	res := FirmwareCheckResult{
		Checked: true,
		Current: curStr,
	}
	body, _ := json.Marshal(map[string]uint32{"dev_id": devID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api2.rongyuan.tech:3816/api/v2/get_fw_version", bytes.NewReader(body))
	if err != nil {
		res.Err = err.Error()
		res.Message = "构造请求失败"
		return res
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 6 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		res.Err = err.Error()
		res.Message = "无法连接官方固件服务器"
		return res
	}
	defer resp.Body.Close()

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			VersionStr string `json:"version_str"`
			Version    string `json:"version"`
			FilePath   string `json:"file_path"`
		} `json:"data"`
		Version string `json:"version"`
		Msg     string `json:"err_message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		res.Err = err.Error()
		res.Message = "解析固件版本响应失败"
		return res
	}

	latest := strings.TrimSpace(apiResp.Data.VersionStr)
	if latest == "" {
		latest = strings.TrimSpace(apiResp.Data.Version)
	}
	if latest == "" {
		latest = strings.TrimSpace(apiResp.Version)
	}
	if latest == "" {
		res.Message = "官方暂未发布更新信息"
		res.UpToDate = true
		return res
	}
	latestClean := strings.TrimPrefix(strings.ToLower(latest), "v")
	curClean := strings.TrimPrefix(strings.ToLower(curStr), "v")
	res.Latest = "v" + latestClean

	var latestNum, curNum uint64
	_, errL := fmt.Sscanf(latestClean, "%x", &latestNum)
	_, errC := fmt.Sscanf(curClean, "%x", &curNum)
	if errL == nil && errC == nil {
		res.UpToDate = curNum >= latestNum
	} else {
		res.UpToDate = strings.TrimLeft(curClean, "0") == strings.TrimLeft(latestClean, "0")
	}

	if res.UpToDate {
		res.Message = fmt.Sprintf("已是最新官方固件 (%s)", res.Latest)
	} else {
		res.Message = fmt.Sprintf("发现新固件 %s（当前 %s），升级请切换至有线 USB 模式", res.Latest, curStr)
	}
	return res
}
