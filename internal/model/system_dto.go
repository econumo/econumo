// Result DTO for the system read endpoint (get-update-info).
package model

// GetUpdateInfoResult is the get-update-info response: the latest release
// published on econumo.com, or empty strings when unknown (check disabled,
// not fetched yet, or the feed unreachable). The SPA compares version against
// its own build version; the server never does.
type GetUpdateInfoResult struct {
	Version string `json:"version"`
	Url     string `json:"url"`
}
