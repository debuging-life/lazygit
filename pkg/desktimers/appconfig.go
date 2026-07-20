package desktimers

import "strings"

// configuredBaseURL is the apiBaseUrl from the dtgit user config
// (desktimers.apiBaseUrl). It sits between the DESKTIMERS_API_URL env var and
// the token file in the base-URL resolution order.
var configuredBaseURL string

// SetConfiguredBaseURL records the apiBaseUrl from the loaded user config.
// An empty or default value clears the override.
func SetConfiguredBaseURL(url string) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if url == DefaultAPIBaseURL {
		url = ""
	}
	configuredBaseURL = url
}
