package desktimers

import "strings"

// configuredBaseURL is the site from the deskgit user config
// (desktimers.url). The DESKTICK_URL env var wins over it.
var configuredBaseURL string

// SetConfiguredBaseURL records the site from the loaded user config. An
// empty, default, or retired value clears the override.
func SetConfiguredBaseURL(url string) {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	if url == DefaultSiteURL || isRetiredBaseURL(url) {
		url = ""
	}
	configuredBaseURL = url
}
