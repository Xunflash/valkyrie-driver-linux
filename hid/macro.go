package hid

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Macro event type strings match buffToMacroEvents / macroEventToByte.
const (
	MacroKeyboard    = "keyboard"
	MacroMouseButton = "mouse_button"
	MacroMouseMove   = "mouse_move"
	MacroDelay       = "delay"
	MacroActionDown  = "down"
	MacroActionUp    = "up"
)

// MacroEvent is one recorded macro step.
// Keyboard / mouse-button / mouse-move events carry KeyCode or DX/DY.
// Delay events carry DelayMs. EncodeMacroBuffer also accepts DelayMs on the
// action itself when the following event is not a separate delay.
type MacroEvent struct {
	Type    string `json:"type"`
	Action  string `json:"action,omitempty"`
	KeyCode uint8  `json:"key_code"`
	DX      int8   `json:"dx"`
	DY      int8   `json:"dy"`
	DelayMs uint16 `json:"delay_ms"`
}

// Macro is one on-mouse macro slot (256-byte packed buffer).
type Macro struct {
	Name        string       `json:"name,omitempty"`
	Slot        uint8        `json:"slot"`
	RepeatCount uint16       `json:"repeat_count"`
	Events      []MacroEvent `json:"events"`
}

// responseOmitsCommandEcho reports Get commands whose 64-byte body does not echo the opcode.
// 0x81 key matrix, 0x83 macro and 0x89 gun return raw payload bytes.
func responseOmitsCommandEcho(cmd byte) bool {
	switch cmd {
	case CmdGetKeyMatrix, CmdGetMacro, CmdGetGun:
		return true
	default:
		return false
	}
}

// EncodeMacroBuffer packs a macro the way setMacro + macroEventToByte do.
// The first two bytes are the little-endian repeat count. Each action is paired
// with a delay (the next "delay" event, or the action's own DelayMs).
func EncodeMacroBuffer(m Macro) ([256]byte, error) {
	var buf [256]byte
	binary.LittleEndian.PutUint16(buf[0:2], m.RepeatCount)
	off := 2
	for i := 0; i < len(m.Events); i++ {
		ev := m.Events[i]
		if ev.Type == MacroDelay {
			return buf, errors.New("宏编码：延迟事件必须跟在动作之后")
		}
		delay := ev.DelayMs
		if i+1 < len(m.Events) && m.Events[i+1].Type == MacroDelay {
			delay = m.Events[i+1].DelayMs
			i++
		}
		chunk, err := macroEventToBytes(ev, delay)
		if err != nil {
			return buf, err
		}
		if off+len(chunk) > len(buf) {
			return buf, errors.New("宏超出 256 字节")
		}
		copy(buf[off:], chunk)
		off += len(chunk)
	}
	return buf, nil
}

func macroEventToBytes(ev MacroEvent, delay uint16) ([]byte, error) {
	switch ev.Type {
	case MacroKeyboard, MacroMouseButton:
		var key byte
		switch ev.Type {
		case MacroKeyboard:
			key = ev.KeyCode
			if key == 0 {
				return nil, errors.New("宏编码：键盘 HID 码无效")
			}
		default:
			key = ev.KeyCode
			if key < 240 || key > 244 {
				return nil, fmt.Errorf("宏编码：鼠标键码无效: %d", key)
			}
		}
		if delay <= 127 {
			b := byte(delay)
			if ev.Action == MacroActionDown {
				b += 128
			}
			return []byte{key, b}, nil
		}
		flag := byte(0)
		if ev.Action == MacroActionDown {
			flag = 128
		}
		return []byte{key, flag, byte(delay), byte(delay >> 8)}, nil
	case MacroMouseMove:
		out := []byte{249, 0, byte(ev.DX), byte(ev.DY)}
		if delay <= 127 {
			out[1] = byte(delay)
			return out, nil
		}
		return append(out, byte(delay), byte(delay>>8)), nil
	default:
		return nil, fmt.Errorf("宏编码：未知事件类型 %q", ev.Type)
	}
}

// DecodeMacroBuffer unpacks a device macro buffer the way buffToMacroEvents does.
// Short buffers are zero-padded to 256 bytes so the trailing [0,0,0,0] terminator
// is visible, matching a 0x83 read. Mouse-move delays stored in a single byte are
// decoded with the official `value >> 1` rule.
func DecodeMacroBuffer(buf []byte) Macro {
	var full [256]byte
	if len(buf) > len(full) {
		buf = buf[:len(full)]
	}
	copy(full[:], buf)
	return decodeMacroPadded(full[:])
}

func decodeMacroPadded(t []byte) Macro {
	m := Macro{RepeatCount: binary.LittleEndian.Uint16(t[0:2])}
	if len(t) < 6 {
		return m
	}
	n := 2
	s := t[n : n+4]
	n += 4
	for n < len(t) {
		if s[0] == 249 {
			m.Events = append(m.Events, MacroEvent{
				Type: MacroMouseMove,
				DX:   int8(s[2]),
				DY:   int8(s[3]),
			})
			var delay uint16
			if s[1] != 0 {
				delay = uint16(s[1] >> 1)
			} else if n+2 <= len(t) {
				delay = binary.LittleEndian.Uint16(t[n : n+2])
				n += 2
			}
			if delay != 0 {
				m.Events = append(m.Events, MacroEvent{Type: MacroDelay, DelayMs: delay})
			}
		} else {
			if s[0] <= 239 && s[0] >= 4 {
				action := MacroActionUp
				if s[1] > 127 {
					action = MacroActionDown
				}
				m.Events = append(m.Events, MacroEvent{Type: MacroKeyboard, Action: action, KeyCode: s[0]})
			} else if s[0] >= 240 && s[0] <= 244 {
				action := MacroActionUp
				if s[1] > 127 {
					action = MacroActionDown
				}
				m.Events = append(m.Events, MacroEvent{Type: MacroMouseButton, Action: action, KeyCode: s[0]})
			}
			var delay uint16
			if s[1]&127 != 0 {
				delay = uint16(s[1] & 127)
				n -= 2
			} else {
				delay = binary.LittleEndian.Uint16(s[2:4])
			}
			if delay != 0 {
				m.Events = append(m.Events, MacroEvent{Type: MacroDelay, DelayMs: delay})
			}
		}
		if n < 0 || n+4 > len(t) {
			break
		}
		s = t[n : n+4]
		if s[0] == 0 && s[1] == 0 && s[2] == 0 && s[3] == 0 {
			break
		}
		n += 4
	}
	return m
}

func macroHasZeroRun(buf []byte) bool {
	for i := 0; i+3 < len(buf); i++ {
		if buf[i] == 0 && buf[i+1] == 0 && buf[i+2] == 0 && buf[i+3] == 0 {
			return true
		}
	}
	return false
}

// macroWritePackets is how many 56-byte chunks _setMacro sends.
// Packets cover index 0 through the last non-zero chunk. An all-zero buffer
// still sends one terminating packet so the slot can be cleared.
func macroWritePackets(buf [256]byte) int {
	last := -1
	for s := 0; s < 5; s++ {
		start := s * 56
		end := start + 56
		if end > len(buf) {
			end = len(buf)
		}
		for _, b := range buf[start:end] {
			if b != 0 {
				last = s
				break
			}
		}
	}
	if last < 0 {
		return 1
	}
	return last + 1
}

// ReadMacro loads one macro slot via 0x83 (up to 4 raw 64-byte packets).
// A packet containing [0,0,0,0] ends the read, matching getMacro.
func (c *Client) ReadMacro(slot uint8) (Macro, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transport == nil {
		return Macro{}, errors.New("设备未连接")
	}
	if slot > 15 {
		return Macro{}, fmt.Errorf("宏槽位超出范围: %d", slot)
	}
	var all []byte
	for n := 0; n < 4; n++ {
		resp, err := c.queryLocked([]byte{CmdGetMacro, slot, byte(n)})
		if err != nil {
			return Macro{}, err
		}
		all = append(all, resp...)
		if macroHasZeroRun(resp) {
			break
		}
	}
	m := DecodeMacroBuffer(all)
	m.Slot = slot
	return m, nil
}

// WriteMacro stores a macro into a slot via 0x03.
// Each packet is an 8-byte header plus 56 payload bytes. The last packet sets hdr[4]=1.
func (c *Client) WriteMacro(slot uint8, m Macro) error {
	if slot > 15 {
		return fmt.Errorf("宏槽位超出范围: %d", slot)
	}
	buf, err := EncodeMacroBuffer(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	packets := macroWritePackets(buf)
	for s := 0; s < packets; s++ {
		payload := make([]byte, 64)
		payload[0] = CmdSetMacro
		payload[1] = slot
		payload[2] = byte(s)
		payload[3] = 56
		if s == packets-1 {
			payload[4] = 1
		}
		start := s * 56
		end := start + 56
		if end > len(buf) {
			end = len(buf)
		}
		copy(payload[8:], buf[start:end])
		if err := c.writeLocked(payload, 0); err != nil {
			return err
		}
	}
	return nil
}
