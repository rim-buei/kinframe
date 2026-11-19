// Package theme describes theme pack layout (Liquid / css-only). Rendering is TODO.
package theme

import (
	"fmt"
	"os"
	"path/filepath"
)

// Type is the pack kind from theme.toml.
type Type string

const (
	TypeFull    Type = "full"
	TypeCSSOnly Type = "css-only"
)

// Meta is the subset of theme.toml used by the scaffold.
type Meta struct {
	ID      string
	Name    string
	Version string
	Engine  string // "liquid"
	Type    Type
}

// LoadMeta reads a minimal theme.toml from themesDir/<id>/theme.toml.
// TODO: parse full TOML and validate templates/ sandbox rules.
func LoadMeta(themesDir, id string) (Meta, error) {
	path := filepath.Join(themesDir, id, "theme.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	// Scaffold: do not fully parse; confirm file exists for operators.
	_ = b
	return Meta{
		ID:      id,
		Name:    id,
		Version: "0.0.0",
		Engine:  "liquid",
		Type:    TypeCSSOnly,
	}, nil
}

// StaticPath returns the on-disk path for a theme static asset (no path traversal).
func StaticPath(themesDir, id, name string) (string, error) {
	base := filepath.Join(themesDir, id, "static")
	cleaned := filepath.Clean("/" + name)
	full := filepath.Join(base, cleaned)
	if rel, err := filepath.Rel(base, full); err != nil || len(rel) >= 2 && rel[:2] == ".." {
		return "", fmt.Errorf("invalid theme static path")
	}
	return full, nil
}
