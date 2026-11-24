package hid

// KeyKind classifies a 4-byte CommonMSCH585 matrix action.
type KeyKind string

const (
	KeyKindMouse   KeyKind = "mouse"
	KeyKindCombo   KeyKind = "combo"
	KeyKindMedia   KeyKind = "media"
	KeyKindFire    KeyKind = "fire"
	KeyKindMacro   KeyKind = "macro"
	KeyKindDisable KeyKind = "disable"
	KeyKindUnknown KeyKind = "unknown"
)

// Macro playback modes stored in byte 1 of a [9, mode, slot, 0] binding.
const (
	MacroModeRepeatTimes = "repeat_times" // 0
	MacroModeOnOff       = "on_off"       // 1
	MacroModeTouchRepeat = "touch_repeat" // 2
)

// HID modifier usage IDs used by combo bindings.
const (
	ModCtrl  uint8 = 224
	ModShift uint8 = 225
	ModAlt   uint8 = 226
	ModWin   uint8 = 227
)

// KeyBinding is a lossless view of one 4-byte key-matrix cell.
type KeyBinding struct {
	Kind KeyKind
	Name string
	Code [4]byte

	// Combo: wire format [0, Mod, Key1, Key2]. Mod/Key1/Key2 are HID usage IDs
	// (Ctrl 224, Shift 225, Alt 226, Win 227, or any key). Up to three codes.
	Mod  uint8
	Key1 uint8
	Key2 uint8

	// Fire: [11, Target, IntervalMs, Count]. Count 0 means repeat while held.
	Target     uint8
	IntervalMs uint8
	Count      uint8

	// Macro: [9, modeByte, MacroSlot, 0].
	MacroMode string
	MacroSlot uint8
}

// HIDKey is one entry in the common keyboard usage table.
type HIDKey struct {
	Name string
	Code uint8
}

// HIDKeys lists common HID keyboard usages (A–Z, digits, F-keys, editing, arrows, punctuation, modifiers).
var HIDKeys = []HIDKey{
	{"A", 4}, {"B", 5}, {"C", 6}, {"D", 7}, {"E", 8}, {"F", 9}, {"G", 10},
	{"H", 11}, {"I", 12}, {"J", 13}, {"K", 14}, {"L", 15}, {"M", 16}, {"N", 17},
	{"O", 18}, {"P", 19}, {"Q", 20}, {"R", 21}, {"S", 22}, {"T", 23}, {"U", 24},
	{"V", 25}, {"W", 26}, {"X", 27}, {"Y", 28}, {"Z", 29},
	{"1", 30}, {"2", 31}, {"3", 32}, {"4", 33}, {"5", 34},
	{"6", 35}, {"7", 36}, {"8", 37}, {"9", 38}, {"0", 39},
	{"Enter", 40}, {"Esc", 41}, {"Backspace", 42}, {"Tab", 43}, {"Space", 44},
	{"Minus", 45}, {"Equal", 46}, {"LeftBracket", 47}, {"RightBracket", 48},
	{"Backslash", 49}, {"Semicolon", 51}, {"Quote", 52}, {"Grave", 53},
	{"Comma", 54}, {"Period", 55}, {"Slash", 56},
	{"CapsLock", 57},
	{"F1", 58}, {"F2", 59}, {"F3", 60}, {"F4", 61}, {"F5", 62}, {"F6", 63},
	{"F7", 64}, {"F8", 65}, {"F9", 66}, {"F10", 67}, {"F11", 68}, {"F12", 69},
	{"PrintScreen", 70},
	{"Insert", 73}, {"Home", 74}, {"PageUp", 75}, {"Delete", 76}, {"End", 77}, {"PageDown", 78},
	{"Right", 79}, {"Left", 80}, {"Down", 81}, {"Up", 82},
	{"Ctrl", ModCtrl}, {"Shift", ModShift}, {"Alt", ModAlt}, {"Win", ModWin},
}

var hidKeyAliases = map[string]uint8{
	"Escape": EscAlias, "Control": ModCtrl, "GUI": ModWin, "Meta": ModWin,
	"ArrowUp": 82, "ArrowDown": 81, "ArrowLeft": 80, "ArrowRight": 79,
	"-": 45, "=": 46, "[": 47, "]": 48, "\\": 49, ";": 51, "'": 52, "`": 53,
	",": 54, ".": 55, "/": 56,
	"PgUp": 75, "PgDn": 78, "Page_Up": 75, "Page_Down": 78,
	"Del": 76, "Return": 40,
}

// EscAlias keeps the alias table referrable without a magic number at the call site.
const EscAlias uint8 = 41

var (
	hidNameToCode map[string]uint8
	hidCodeToName map[uint8]string
)

func init() {
	hidNameToCode = make(map[string]uint8, len(HIDKeys)+len(hidKeyAliases))
	hidCodeToName = make(map[uint8]string, len(HIDKeys))
	for _, k := range HIDKeys {
		hidNameToCode[k.Name] = k.Code
		if _, ok := hidCodeToName[k.Code]; !ok {
			hidCodeToName[k.Code] = k.Name
		}
	}
	for name, code := range hidKeyAliases {
		if _, ok := hidNameToCode[name]; !ok {
			hidNameToCode[name] = code
		}
	}
}

// HIDKeyCode resolves a key name (or alias) to its HID usage ID.
func HIDKeyCode(name string) (uint8, bool) {
	c, ok := hidNameToCode[name]
	return c, ok
}

// HIDKeyName resolves a HID usage ID to its primary name.
func HIDKeyName(code uint8) (string, bool) {
	n, ok := hidCodeToName[code]
	return n, ok
}

type namedCode struct {
	Name string
	Code [4]byte
}

// Canonical mouse functions. DPI- encodes as [20,0,2,0]; [0,2,0,20] is accepted on decode only.
var mouseActions = []namedCode{
	{"左键", [4]byte{1, 0, 240, 0}},
	{"右键", [4]byte{1, 0, 241, 0}},
	{"中键", [4]byte{1, 0, 242, 0}},
	{"前进", [4]byte{1, 0, 244, 0}},
	{"后退", [4]byte{1, 0, 243, 0}},
	{"滚轮向上", [4]byte{1, 0, 245, 1}},
	{"滚轮向下", [4]byte{1, 0, 245, 255}},
	{"滚轮向左", [4]byte{1, 0, 245, 0}},
	{"滚轮向右", [4]byte{1, 0, 246, 0}},
	{"DPI循环", [4]byte{20, 0, 0, 0}},
	{"DPI+", [4]byte{20, 0, 1, 0}},
	{"DPI-", [4]byte{20, 0, 2, 0}},
	{"DPI狙击", [4]byte{20, 0, 4, 0}},
	{"配置循环", [4]byte{8, 0, 3, 0}},
	{"配置+", [4]byte{8, 0, 1, 0}},
	{"配置-", [4]byte{8, 0, 2, 0}},
}

// Legacy DPI- matrix used by older presets. Decode accepts it; encode writes [20,0,2,0].
var dpiMinusLegacy = [4]byte{0, 2, 0, 20}

// Media and system functions from the official hv table (plus the Zo shortcuts the UI treats as media).
var mediaActions = []namedCode{
	{"音量加", [4]byte{3, 0, 233, 0}},
	{"音量减", [4]byte{3, 0, 234, 0}},
	{"静音", [4]byte{3, 0, 226, 0}},
	{"播放/暂停", [4]byte{3, 0, 205, 0}},
	{"上一曲", [4]byte{3, 0, 182, 0}},
	{"下一曲", [4]byte{3, 0, 181, 0}},
	{"停止", [4]byte{3, 0, 183, 0}},
	{"播放器", [4]byte{3, 0, 131, 1}},
	{"计算器", [4]byte{3, 0, 146, 1}},
	{"我的电脑", [4]byte{3, 0, 148, 1}},
	{"邮件", [4]byte{3, 0, 138, 1}},
	{"浏览器主页", [4]byte{3, 0, 35, 2}},
	{"搜索", [4]byte{3, 0, 33, 2}},
	{"返回", [4]byte{3, 0, 36, 2}},
	{"刷新", [4]byte{3, 0, 39, 2}},
	{"亮度加", [4]byte{3, 0, 111, 0}},
	{"亮度减", [4]byte{3, 0, 112, 0}},
	{"锁屏", [4]byte{0, 0, 227, 15}},
	{"显示桌面", [4]byte{0, 0, 227, 7}},
	{"截屏", [4]byte{0, 0, 70, 0}},
	{"复制", [4]byte{0, 0, 224, 6}},
	{"粘贴", [4]byte{0, 0, 224, 25}},
	{"剪切", [4]byte{0, 0, 224, 27}},
	{"系统睡眠", [4]byte{2, 0, 130, 0}},
}

func lookupNamed(table []namedCode, code [4]byte) (string, bool) {
	for _, item := range table {
		if item.Code == code {
			return item.Name, true
		}
	}
	return "", false
}

func codeByName(table []namedCode, name string) ([4]byte, bool) {
	for _, item := range table {
		if item.Name == name {
			return item.Code, true
		}
	}
	return [4]byte{}, false
}

// DecodeKeyAction converts a 4-byte matrix cell into a structured binding.
// The legacy DPI- code [0,2,0,20] is recognized as DPI-.
func DecodeKeyAction(code [4]byte) KeyBinding {
	if code == [4]byte{} {
		return KeyBinding{Kind: KeyKindDisable, Name: "禁用按键", Code: code}
	}
	if code == dpiMinusLegacy {
		canon := [4]byte{20, 0, 2, 0}
		return KeyBinding{Kind: KeyKindMouse, Name: "DPI-", Code: canon}
	}
	if name, ok := lookupNamed(mouseActions, code); ok {
		return KeyBinding{Kind: KeyKindMouse, Name: name, Code: code}
	}
	if name, ok := lookupNamed(mediaActions, code); ok {
		return KeyBinding{Kind: KeyKindMedia, Name: name, Code: code}
	}
	switch code[0] {
	case 11:
		return KeyBinding{
			Kind:       KeyKindFire,
			Name:       "火力键",
			Code:       code,
			Target:     code[1],
			IntervalMs: code[2],
			Count:      code[3],
		}
	case 9:
		mode := MacroModeRepeatTimes
		switch code[1] {
		case 1:
			mode = MacroModeOnOff
		case 2:
			mode = MacroModeTouchRepeat
		}
		return KeyBinding{
			Kind:      KeyKindMacro,
			Name:      "宏",
			Code:      code,
			MacroMode: mode,
			MacroSlot: code[2],
		}
	case 0:
		return KeyBinding{
			Kind: KeyKindCombo,
			Name: "组合键",
			Code: code,
			Mod:  code[1],
			Key1: code[2],
			Key2: code[3],
		}
	default:
		return KeyBinding{Kind: KeyKindUnknown, Name: "未知", Code: code}
	}
}

// EncodeKeyAction converts a binding back to its 4-byte matrix cell.
func EncodeKeyAction(b KeyBinding) [4]byte {
	switch b.Kind {
	case KeyKindDisable:
		return [4]byte{}
	case KeyKindMouse:
		if b.Name == "DPI-" || b.Code == dpiMinusLegacy {
			return [4]byte{20, 0, 2, 0}
		}
		if nameCode, ok := codeByName(mouseActions, b.Name); ok {
			return nameCode
		}
		if b.Code != [4]byte{} {
			return b.Code
		}
		return [4]byte{}
	case KeyKindMedia:
		if nameCode, ok := codeByName(mediaActions, b.Name); ok {
			return nameCode
		}
		return b.Code
	case KeyKindCombo:
		return [4]byte{0, b.Mod, b.Key1, b.Key2}
	case KeyKindFire:
		// Target 0 is preserved. FireKey() supplies the left-button default (240).
		return [4]byte{11, b.Target, b.IntervalMs, b.Count}
	case KeyKindMacro:
		var mode byte
		switch b.MacroMode {
		case MacroModeOnOff:
			mode = 1
		case MacroModeTouchRepeat:
			mode = 2
		default:
			mode = 0
		}
		return [4]byte{9, mode, b.MacroSlot, 0}
	default:
		return b.Code
	}
}

// FireKey builds a rapid-fire binding. target 0 selects the left button (240).
// intervalMs is 1..255. count 0 repeats while the button is held.
func FireKey(target, intervalMs, count uint8) KeyBinding {
	if target == 0 {
		target = 240
	}
	b := KeyBinding{
		Kind:       KeyKindFire,
		Name:       "火力键",
		Target:     target,
		IntervalMs: intervalMs,
		Count:      count,
	}
	b.Code = EncodeKeyAction(b)
	return b
}

// NamedKeyAction pairs a display label with its canonical 4-byte matrix code.
type NamedKeyAction struct {
	Name string
	Code [4]byte
}

// MouseActions returns all canonical mouse / DPI / profile actions.
func MouseActions() []NamedKeyAction {
	out := make([]NamedKeyAction, len(mouseActions))
	for i, a := range mouseActions {
		out[i] = NamedKeyAction{Name: a.Name, Code: a.Code}
	}
	return out
}

// MediaActions returns all canonical multimedia & system shortcut actions.
func MediaActions() []NamedKeyAction {
	out := make([]NamedKeyAction, len(mediaActions))
	for i, a := range mediaActions {
		out[i] = NamedKeyAction{Name: a.Name, Code: a.Code}
	}
	return out
}

// BindingSummary returns a concise Chinese display label for any 4-byte key matrix entry.
func BindingSummary(code [4]byte) string {
	b := DecodeKeyAction(code)
	switch b.Kind {
	case KeyKindDisable:
		return "已禁用"
	case KeyKindMouse, KeyKindMedia:
		return b.Name
	case KeyKindFire:
		target := "左键"
		switch b.Target {
		case 240, 0:
			target = "左键"
		case 241:
			target = "右键"
		case 242:
			target = "中键"
		default:
			if n, ok := HIDKeyName(b.Target); ok {
				target = n
			}
		}
		if b.Count == 0 {
			return "火力:" + target + " 持续"
		}
		return "火力:" + target
	case KeyKindMacro:
		mode := "次数"
		switch b.MacroMode {
		case MacroModeOnOff:
			mode = "开关"
		case MacroModeTouchRepeat:
			mode = "按住"
		}
		return "宏#" + string(rune('1'+(b.MacroSlot%9))) + " (" + mode + ")"
	case KeyKindCombo:
		parts := make([]string, 0, 3)
		for _, c := range []uint8{b.Mod, b.Key1, b.Key2} {
			if c == 0 {
				continue
			}
			if n, ok := HIDKeyName(c); ok {
				parts = append(parts, n)
			}
		}
		if len(parts) > 0 {
			s := parts[0]
			for _, p := range parts[1:] {
				s += " + " + p
			}
			return s
		}
		return "组合键"
	default:
		return LabelForKeyCode(code)
	}
}

// ComboKeys builds a combo binding from up to three HID usage IDs
// (modifiers and/or keys), stored as [0, a, b, c].
func ComboKeys(codes ...uint8) KeyBinding {
	b := KeyBinding{Kind: KeyKindCombo, Name: "组合键"}
	if len(codes) > 0 {
		b.Mod = codes[0]
	}
	if len(codes) > 1 {
		b.Key1 = codes[1]
	}
	if len(codes) > 2 {
		b.Key2 = codes[2]
	}
	b.Code = EncodeKeyAction(b)
	return b
}
