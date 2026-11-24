package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/Xunflash/valkyrie-driver-linux/hid"
)

//go:embed assets/icon.png
var appIconPNG []byte

func main() {
	statusFlag := flag.Bool("status", false, "以只读模式读取鼠标实时状态并打印后退出")
	jsonFlag := flag.Bool("json", false, "配合 --status 输出 JSON 格式")
	checkFWFlag := flag.Bool("check-fw", false, "配合 --status 在线检查最新固件版本")
	flag.Parse()

	backend := NewLinuxHIDBackend()

	if *statusFlag {
		os.Exit(runCLIStatus(backend, *jsonFlag, *checkFWFlag))
	}

	store, err := NewLocalStore("")
	if err != nil {
		log.Printf("警告: 初始化本地配置存储失败: %v", err)
	}

	mygo.App.SetName("Valkyrie Driver")
	mygo.App.SetVersion("2.0.0")

	app := NewDriverApp(backend, store)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Valkyrie Studio · VK M5",
			Width:           1140,
			Height:          760,
			MinWidth:        940,
			MinHeight:       620,
			StateKey:        "vk-m5-studio-v2",
			BackgroundColor: "light-dark(#f2f5fa, #080b11)",
			Content:         ui.View(app.View),
		})
		if len(appIconPNG) > 0 {
			_ = win.SetIcon(appIconPNG)
		}
		app.AttachWindow(win)
		app.StartBackground()
	})

	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		app.Stop()
		_ = backend.Close()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func runCLIStatus(backend Backend, asJSON bool, checkFW bool) int {
	defer backend.Close()
	st, err := backend.Connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}

	var fwRes *hid.FirmwareCheckResult
	if checkFW {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		r := backend.CheckFirmware(ctx, st.DevID, st.FirmwareVer)
		cancel()
		fwRes = &r
	}

	if asJSON {
		out := map[string]any{
			"state": st,
		}
		if fwRes != nil {
			out["firmware_check"] = fwRes
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Printf("设备型号: %s (%s) [ID=%d]\n", st.Spec.Model, st.Spec.Sensor, st.DevID)
	fmt.Printf("连接方式: %s · %s (%s)\n", st.Info.Mode.Label(), st.Info.Path, st.Info.VIDPID())
	fmt.Printf("固件版本: %s", st.FirmwareString())
	if fwRes != nil {
		fmt.Printf(" · %s", fwRes.Message)
	}
	fmt.Println()
	if st.Battery >= 0 {
		fmt.Printf("当前电量: %d%%\n", st.Battery)
	}
	fmt.Printf("板载配置: 配置 %d\n", st.Params.Profile+1)
	fmt.Printf("回报率: %d Hz | 去抖: %d ms | 滚轮防抖: %d ms | 真8K: %v\n",
		st.Params.ReportRate, st.Params.Debounce, st.Params.ScrollUpTime, st.Params.True8K)
	lodLabel := fmt.Sprintf("档位 %d", st.Params.SilentHeight)
	if int(st.Params.SilentHeight) < len(st.Spec.LODOptions) {
		lodLabel = st.Spec.LODOptions[st.Params.SilentHeight]
	}
	fmt.Printf("静默高度: %s | 角度微调: %+d° | MotionSync: %v | 直线修正: %v | 波纹修正: %v | 20K FPS: %v | FastTap: %v\n",
		lodLabel, st.Params.AngleTune, st.Params.MotionSync, st.Params.LineRepair,
		st.Params.WaveRepair, st.Params.FPS20000, st.Params.LowLatency)
	fmt.Printf("休眠时间: 2.4G=%ds, 蓝牙=%ds\n", st.Params.Sleep24G, st.Params.SleepBT)
	fmt.Println("DPI 档位:")
	for i, d := range st.DPI.Stages {
		mark := " "
		if i == st.DPI.Current {
			mark = "*"
		}
		fmt.Printf("  %s 档位 %d: X=%-5d Y=%-5d  #%02X%02X%02X\n", mark, i+1, d.X, d.Y, d.R, d.G, d.B)
	}
	fmt.Println("按键映射:")
	for _, slot := range hid.ConfigurableSlots {
		fmt.Printf("  %-14s -> %s\n", slot.Name, hid.BindingSummary(st.Keys[slot.Index]))
	}
	return 0
}
