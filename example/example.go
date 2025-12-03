package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/atotto/clipboard"
	hook "github.com/robotn/gohook"
)

func main() {
    a := app.New()
    win := a.NewWindow("Clipboard → Input Demo (Alt+Q)")
    win.Resize(fyne.NewSize(600, 140))

    input := widget.NewMultiLineEntry()
    input.SetPlaceHolder("先选中文本 → Ctrl+C → 按 Alt+Q 来粘贴剪贴板内容")

    content := container.NewVBox(
        widget.NewLabel("选中文本（浏览器）→ Ctrl+C → Alt+Q"),
        input,
    )
    win.SetContent(content)
    
    // 注册全局热键 Alt+Q
    hook.Register(hook.KeyDown, []string{"q", "alt"}, func(e hook.Event) {
		fmt.Println("按下 Alt+Q，准备粘贴剪贴板内容...")
        txt, err := clipboard.ReadAll()
        if err != nil {
            log.Println("读取剪贴板失败:", err)
            return
        }
        fyne.Do(func() {
            input.SetText(txt)
            win.Canvas().Focus(input)
            input.Refresh()
        })
    })
	hook.Register(hook.MouseMove, []string{}, func(e hook.Event) {
        fmt.Printf("MouseMove: %+v\n", e)
    })
    // 启动 hook 消息循环
	go func() {
		s := hook.Start()
		<-hook.Process(s)
	}()
    defer hook.End()
	
	win.ShowAndRun()
	
    // 捕获退出信号
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
    <-sig

    win.Close()
    a.Quit()
    
}
