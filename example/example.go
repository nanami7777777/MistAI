package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Fyne 性能测试")
	myWindow.Resize(fyne.NewSize(800, 600))

	start := time.Now()

	numButtons := 1000
	buttons := make([]fyne.CanvasObject, numButtons)
	for i := 0; i < numButtons; i++ {
		btn := widget.NewButton(fmt.Sprintf("Btn %d", i+1), nil)
		buttons[i] = btn
	}

	content := container.NewGridWrap(fyne.NewSize(120, 40), buttons...)

	elapsed := time.Since(start)
	fmt.Printf("✅ 创建 %d 个按钮耗时: %v\n", numButtons, elapsed)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}