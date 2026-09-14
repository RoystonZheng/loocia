package tools

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

func validateOptionalCooperURL(raw string) (*string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if err := validateCooperURL(raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func validateCooperURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() != "cooper.didichuxing.com" {
		return &DiscoveryError{Class: ErrorInvalidCooperURL, Message: "Cooper URL must belong to https://cooper.didichuxing.com/"}
	}
	return nil
}

func requireText(v, field string, maxLen int) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", &DiscoveryError{Class: ErrorValidation, Message: field + " is required"}
	}
	if maxLen > 0 && utf8.RuneCountInString(v) > maxLen {
		return "", &DiscoveryError{Class: ErrorValidation, Message: field + " is too long"}
	}
	return v, nil
}

func requireTextRange(v, field string, minLen, maxLen int) (string, error) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if n < minLen {
		return "", &DiscoveryError{Class: ErrorValidation, Message: field + " is too short"}
	}
	if maxLen > 0 && n > maxLen {
		return "", &DiscoveryError{Class: ErrorValidation, Message: field + " is too long"}
	}
	return v, nil
}
