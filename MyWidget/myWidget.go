package mywidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

)

type MyMultiLine struct {
	widget.Entry
	OnEnter func()
}

func NewMyMultiLine(onEnter func()) *MyMultiLine {
	e := &MyMultiLine{
		OnEnter: onEnter,
	}
	e.MultiLine = true    // promoted field
	e.ExtendBaseWidget(e) // registers widget correctly
	return e
}

func (m *MyMultiLine) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyReturn {
		if m.OnEnter != nil {
			m.OnEnter()
		}
	}
	m.Entry.TypedKey(k) // call base behav ior
}
