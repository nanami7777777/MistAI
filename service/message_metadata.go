package service

import (
	"fyne.io/fyne/v2"
	"sync"
)

// Global map to store message metadata for efficient lookup
var MessageMetadata = make(map[*fyne.Container]Metadata)
var MetadataMu sync.RWMutex

// Metadata stores metadata for a message widget
type Metadata struct {
	ID      uint
	Sender  string
	Content string
}
