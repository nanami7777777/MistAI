package mywidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type HoverableLabel struct {
	widget.Label
	OnHoverIn      func()
	OnHoverOut     func()
	OnDoubleTapped func()
	OnTapped       func()
	OnRightTapped  func(*fyne.PointEvent)
}

// NewHoverableLabel creates a new HoverableLabel.
func NewHoverableLabel(text string) *HoverableLabel {
	l := &HoverableLabel{}
	l.ExtendBaseWidget(l)
	l.SetText(text)
	return l
}

// MouseIn is called when the mouse enters the widget.
func (l *HoverableLabel) MouseIn(_ *desktop.MouseEvent) {
	if l.OnHoverIn != nil {
		l.OnHoverIn()
	}
}

// MouseOut is called when the mouse leaves the widget.
func (l *HoverableLabel) MouseOut() {
	if l.OnHoverOut != nil {
		l.OnHoverOut()
	}
}

// MouseMoved is called when the mouse moves over the widget.
func (l *HoverableLabel) MouseMoved(_ *desktop.MouseEvent) {}

// DoubleTapped is called when the user double-taps the widget.
func (l *HoverableLabel) DoubleTapped(_ *fyne.PointEvent) {
	if l.OnDoubleTapped != nil {
		l.OnDoubleTapped()
	}
}

func (l *HoverableLabel) Tapped(_ *fyne.PointEvent) {
	if l.OnTapped != nil {
		l.OnTapped()
	}
}

func (l *HoverableLabel) TappedSecondary(ev *fyne.PointEvent) {
	if l.OnRightTapped != nil {
		l.OnRightTapped(ev)
	}
}

type MyMultiLine struct {
	widget.Entry
	OnSubmit  func()
	shiftDown bool
}

func NewMyMultiLine(submit func()) *MyMultiLine {
	entry := &MyMultiLine{OnSubmit: submit}
	entry.MultiLine = true
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *MyMultiLine) TypedKey(ev *fyne.KeyEvent) {
	if ev.Name == fyne.KeyReturn || ev.Name == fyne.KeyEnter {
		if e.shiftDown {
			e.Entry.TypedKey(ev)
		} else if e.OnSubmit != nil {
			e.OnSubmit()
		}
		return
	}
	e.Entry.TypedKey(ev)
}

func (e *MyMultiLine) KeyDown(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		e.shiftDown = true
	}
	e.Entry.KeyDown(ev)
}

func (e *MyMultiLine) KeyUp(ev *fyne.KeyEvent) {
	if ev.Name == desktop.KeyShiftLeft || ev.Name == desktop.KeyShiftRight {
		e.shiftDown = false
	}
	e.Entry.KeyUp(ev)
}

type ConversationItem struct {
	widget.BaseWidget
	Label         *widget.Label
	OnTapped      func()
	OnRightTapped func(*fyne.PointEvent)
}

func NewConversationItem(text string, bold bool) *ConversationItem {
	item := &ConversationItem{
		Label: widget.NewLabel(text),
	}
	if bold {
		item.Label.TextStyle = fyne.TextStyle{Bold: true}
	}
	item.ExtendBaseWidget(item)
	return item
}

func (c *ConversationItem) CreateRenderer() fyne.WidgetRenderer {
	box := container.NewHBox(c.Label)
	return widget.NewSimpleRenderer(box)
}

func (c *ConversationItem) Tapped(_ *fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped()
	}
}

func (c *ConversationItem) TappedSecondary(ev *fyne.PointEvent) {
	if c.OnRightTapped != nil {
		c.OnRightTapped(ev)
	}
}
