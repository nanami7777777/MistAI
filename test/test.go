package main

import (
    "fmt"
    hook "github.com/robotn/gohook"
)

func main() {
    fmt.Println("Starting global hook. Press some keys / move mouse ... Ctrl+Shift+Q to quit.")
    // 注册一个热键 Ctrl+Shift+Q 来退出
    hook.Register(hook.KeyDown, []string{"q", "ctrl", "shift"}, func(e hook.Event) {
        fmt.Println("Detected Ctrl+Shift+Q — exiting.")
        hook.End()
    })

    // 同时，也打印所有按键按下 /抬起事件
    hook.Register(hook.KeyDown, []string{}, func(e hook.Event) {
        fmt.Printf("KeyDown: %+v\n", e)
    })
    hook.Register(hook.KeyUp, []string{}, func(e hook.Event) {
        fmt.Printf("KeyUp  : %+v\n", e)
    })

    // 如果你还想监控鼠标移动 /点击，也可以
    hook.Register(hook.MouseMove, []string{}, func(e hook.Event) {
        fmt.Printf("MouseMove: %+v\n", e)
    })
    hook.Register(hook.MouseDown, []string{}, func(e hook.Event) {
        fmt.Printf("MouseDown: %+v\n", e)
    })
    hook.Register(hook.MouseUp, []string{}, func(e hook.Event) {
        fmt.Printf("MouseUp  : %+v\n", e)
    })

    // 启动 hook
    s := hook.Start()
    <-hook.Process(s)
    fmt.Println("Hook ended, program exiting.")
}
