package utm

import (
	"fmt"
	"net/url"
	"strings"
)

// Params contains optional UTM campaign parameters.
type Params struct {
	Source   string `json:"utm_source,omitempty"`
	Medium   string `json:"utm_medium,omitempty"`
	Campaign string `json:"utm_campaign,omitempty"`
	Term     string `json:"utm_term,omitempty"`
	Content  string `json:"utm_content,omitempty"`
}

// Build appends the supplied UTM parameters to an absolute HTTP(S) URL.
// Existing query parameters are preserved. Empty UTM values are ignored.
func Build(raw string, p Params) (string, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url must be an absolute http or https URL")
	}

	q := u.Query()
	for key, value := range map[string]string{
		"utm_source": p.Source,
		"utm_medium": p.Medium,
		"utm_campaign": p.Campaign,
		"utm_term": p.Term,
		"utm_content": p.Content,
	} {
		if strings.TrimSpace(value) != "" {
			q.Set(key, strings.TrimSpace(value))
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
