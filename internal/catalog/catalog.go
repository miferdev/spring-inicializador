// Package catalog parses the Spring Initializr client metadata document into a
// plain value type. It performs no I/O and never sorts: the catalog keeps the
// server's order, and display ordering (dependency count descending, ties
// alphabetical, decision D7) belongs to the UI layer.
package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Choice struct {
	ID   string
	Name string
}

// Select mirrors the metadata's single-select / action fields.
type Select struct {
	Default string
	Values  []Choice
}

type Link struct {
	Title string // may be empty; some links are bare YouTube URLs with no title
	Href  string
}

type Dependency struct {
	ID          string
	Name        string
	Description string
	Category    string
	Guides      []Link
}

type Category struct {
	Name         string
	Dependencies []*Dependency
}

type Catalog struct {
	Categories   []Category   // server order, never sorted
	Dependencies []Dependency // flat, server order: category order, then item order
	Type         Select
	Packaging    Select
	JavaVersion  Select
	Language     Select
	BootVersion  Select
	Text         map[string]string // groupId, artifactId, version, name, description, packageName defaults

	index map[string]int
}

// Wire structs mirror the document shape and stay unexported so the public
// types are never coupled to the encoding details.
type wireCatalog struct {
	Dependencies wireDependencies `json:"dependencies"`
	Type         wireSelect       `json:"type"`
	Packaging    wireSelect       `json:"packaging"`
	JavaVersion  wireSelect       `json:"javaVersion"`
	Language     wireSelect       `json:"language"`
	BootVersion  wireSelect       `json:"bootVersion"`
	GroupID      wireText         `json:"groupId"`
	ArtifactID   wireText         `json:"artifactId"`
	Version      wireText         `json:"version"`
	Name         wireText         `json:"name"`
	Description  wireText         `json:"description"`
	PackageName  wireText         `json:"packageName"`
}

type wireDependencies struct {
	Values []wireCategory `json:"values"`
}

type wireCategory struct {
	Name   string           `json:"name"`
	Values []wireDependency `json:"values"`
}

type wireDependency struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Links       *wireLinks `json:"_links"`
}

type wireLinks struct {
	Guide json.RawMessage `json:"guide"`
}

type wireLink struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

type wireSelect struct {
	Default string       `json:"default"`
	Values  []wireChoice `json:"values"`
}

type wireChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireText struct {
	Default string `json:"default"`
}

// Parse decodes a metadata/client response. It performs no I/O and does not
// sort: category and dependency order is exactly the server's. Display ordering
// (dependency count descending, ties alphabetical, decision D7) belongs to the
// UI layer. Unknown fields are ignored and a document without categories is
// still a valid catalog.
func Parse(data []byte) (*Catalog, error) {
	var wire wireCatalog
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("catalog: parsing metadata: %w", err)
	}

	c := &Catalog{
		Type:        selectFromWire(wire.Type),
		Packaging:   selectFromWire(wire.Packaging),
		JavaVersion: selectFromWire(wire.JavaVersion),
		Language:    selectFromWire(wire.Language),
		BootVersion: selectFromWire(wire.BootVersion),
		Text: map[string]string{
			"groupId":     wire.GroupID.Default,
			"artifactId":  wire.ArtifactID.Default,
			"version":     wire.Version.Default,
			"name":        wire.Name.Default,
			"description": wire.Description.Default,
			"packageName": wire.PackageName.Default,
		},
		index: make(map[string]int),
	}

	// Category.Dependencies holds pointers into the flat Dependencies slice, so
	// that slice must be final before any pointer is taken. Record membership
	// by index during this pass and resolve it into pointers afterwards.
	type membership struct {
		name      string
		positions []int
	}
	groups := make([]membership, 0, len(wire.Dependencies.Values))
	seen := make(map[string]struct{})

	for _, wc := range wire.Dependencies.Values {
		group := membership{name: wc.Name}
		for _, wd := range wc.Values {
			// Duplicate dependency IDs keep the first occurrence. Later copies
			// are dropped from both the flat slice and their category, so every
			// dependency has exactly one stable pointer and one owning category.
			if _, ok := seen[wd.ID]; ok {
				continue
			}
			seen[wd.ID] = struct{}{}

			guides, err := guidesFromWire(wd.Links)
			if err != nil {
				return nil, fmt.Errorf("catalog: parsing metadata: dependency %q: %w", wd.ID, err)
			}

			position := len(c.Dependencies)
			// An empty id is not an addressable dependency, so it is stored
			// but never indexed; Lookup("") always misses.
			if wd.ID != "" {
				c.index[wd.ID] = position
			}
			c.Dependencies = append(c.Dependencies, Dependency{
				ID:          wd.ID,
				Name:        wd.Name,
				Description: wd.Description,
				Category:    wc.Name,
				Guides:      guides,
			})
			group.positions = append(group.positions, position)
		}
		groups = append(groups, group)
	}

	// Dependencies is final now; only here is it safe to point into it.
	c.Categories = make([]Category, 0, len(groups))
	for _, group := range groups {
		category := Category{
			Name:         group.name,
			Dependencies: make([]*Dependency, 0, len(group.positions)),
		}
		for _, position := range group.positions {
			category.Dependencies = append(category.Dependencies, &c.Dependencies[position])
		}
		c.Categories = append(c.Categories, category)
	}

	return c, nil
}

// Lookup returns the dependency registered under id, if any. It is O(1)
// through the index built by Parse and returns a copy of the dependency value.
// The zero-value Catalog and the empty id never match a dependency.
func (c *Catalog) Lookup(id string) (Dependency, bool) {
	if c == nil {
		return Dependency{}, false
	}
	position, ok := c.index[id]
	if !ok || position < 0 || position >= len(c.Dependencies) {
		return Dependency{}, false
	}
	return c.Dependencies[position], true
}

func selectFromWire(w wireSelect) Select {
	sel := Select{
		Default: w.Default,
		Values:  make([]Choice, 0, len(w.Values)),
	}
	for _, value := range w.Values {
		sel.Values = append(sel.Values, Choice{ID: value.ID, Name: value.Name})
	}
	return sel
}

// guidesFromWire decodes the three shapes the server uses for _links.guide:
// absent, a single object, and an array of objects. Title is optional inside
// either shape, while href is expected.
func guidesFromWire(links *wireLinks) ([]Link, error) {
	if links == nil {
		return nil, nil
	}

	raw := bytes.TrimSpace(links.Guide)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var wires []wireLink
		if err := json.Unmarshal(raw, &wires); err != nil {
			return nil, fmt.Errorf("decoding guide links: %w", err)
		}
		return linksFromWire(wires), nil
	case '{':
		var wire wireLink
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, fmt.Errorf("decoding guide link: %w", err)
		}
		return linksFromWire([]wireLink{wire}), nil
	default:
		return nil, fmt.Errorf("unexpected guide encoding: %.24q", raw)
	}
}

func linksFromWire(wires []wireLink) []Link {
	if len(wires) == 0 {
		return nil
	}
	links := make([]Link, 0, len(wires))
	for _, wire := range wires {
		links = append(links, Link{Title: wire.Title, Href: wire.Href})
	}
	return links
}
