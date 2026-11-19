package space

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var slugClean = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a display name into a URL slug candidate.
func Slugify(name string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return '-'
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(name))
	s = slugClean.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "space"
	}
	if len(s) > 64 {
		s = s[:64]
		s = strings.Trim(s, "-")
	}
	return s
}

// UniqueSlug appends -2, -3, ... until availableFn returns true.
func UniqueSlug(base string, availableFn func(slug string) (bool, error)) (string, error) {
	base = Slugify(base)
	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		ok, err := availableFn(candidate)
		if err != nil {
			return "", err
		}
		if ok {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate unique slug")
}
