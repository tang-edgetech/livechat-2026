// Package htmlguard is a cheap defense-in-depth check for HTML-flagged
// Canned Message / Automation Rule text. The real enforcement point is
// the frontend's DOMPurify render pass (sanitizeHtml.ts) — this only
// catches accidental/obvious payloads at save time, not a substitute
// for sanitizing on render.
package htmlguard

import "regexp"

var dangerousPatterns = regexp.MustCompile(`(?i)<script|on\w+\s*=|javascript:`)

func IsDangerous(html string) bool {
	return dangerousPatterns.MatchString(html)
}
