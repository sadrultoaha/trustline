package ai

import "regexp"

// Redaction removes personal data and secrets before text leaves the platform.
// Emails become role-preserving placeholders so the model can still reason
// about "a user" without learning who it is.
var redactors = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`), "[email]"},
	{regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`), "[aws-access-key]"},
	{regexp.MustCompile(`\b(ghp|gho|ghs|github_pat)_[A-Za-z0-9_]{20,}\b`), "[github-token]"},
	{regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), "[ssn]"},
	{regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`), "[card-number]"},
	{regexp.MustCompile(`\+?\d{1,3}[ .-]?\(?\d{2,4}\)?[ .-]?\d{3,4}[ .-]?\d{3,4}\b`), "[phone]"},
}

// Redact scrubs PII and secrets from s.
func Redact(s string) string {
	for _, r := range redactors {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
