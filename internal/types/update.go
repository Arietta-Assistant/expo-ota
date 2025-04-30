package types

import (
	"time"
)

type UpdateType int

const (
	UpdateTypeNone UpdateType = iota
	UpdateTypeNormal
	UpdateTypeRollback
)

type Update struct {
	Branch         string        `json:"branch"`
	RuntimeVersion string        `json:"runtimeVersion"`
	UpdateId       string        `json:"updateId"`
	CreatedAt      time.Duration `json:"createdAt"`
	Active         bool          `json:"active"`
	Platform       string        `json:"platform,omitempty"`
	CommitHash     string        `json:"commitHash,omitempty"`
}

type FileUpdateRequest struct {
	Url  string `json:"url"`
	Path string `json:"path"`
}

// Add any other existing types if needed
