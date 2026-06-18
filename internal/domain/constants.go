package domain

// Core application constants
const (
	AppName    = "quickcull"
	AppVersion = "1.0.1"
	MaxLabel   = 5 // Labels are 1–5 inclusive; 0 means unlabelled
)

// Centralized folder names for application and output structure
const (
	DirPhotos     = "photos"
	DirEvents     = ".events"
	DirDuplicates = ".duplicates"
	DirNoDate     = "no-date"
	DirTrash      = ".trash"
)

// UndoActionType defines the type of action that can be undone.
type UndoActionType int

const (
	ActionTrash UndoActionType = iota
	ActionStar
	ActionRotate
	ActionLabel
)
