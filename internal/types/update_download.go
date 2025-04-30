package types

// UpdateDownload represents a record of a user downloading an update
type UpdateDownload struct {
	UpdateId       string `json:"updateId"`
	UserId         string `json:"userId"`
	DeviceId       string `json:"deviceId"`
	Platform       string `json:"platform"`
	DownloadedAt   string `json:"downloadedAt"`
	RuntimeVersion string `json:"runtimeVersion"`
	Branch         string `json:"branch"`
}
