package desktimers

import "strings"

// configuredBaseURL is the apiBaseUrl from the deskgit user config
// (desktimers.apiBaseUrl). It sits between the DESKTIMERS_API_URL env var and
// the token file in the base-URL resolution order.
var configuredBaseURL string

// SetConfiguredBaseURL records the apiBaseUrl from the loaded user config. An
// empty, default, or retired value clears the override — the last so a config
// copied from the old README sample (which pinned a now-dead host) falls
// through to the current default instead of overriding it.
func SetConfiguredBaseURL(url string) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if url == DefaultAPIBaseURL || isRetiredBaseURL(url) {
		url = ""
	}
	configuredBaseURL = url
}
