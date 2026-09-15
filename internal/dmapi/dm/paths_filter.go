package dm

import (
	"strings"

	"github.com/rs/zerolog/log"
)

type PathsFilter struct {
	findDirectChildren func(string) []string
	filteredPaths      map[string]bool // true hides a type; false is a visible exception
}

func NewPathsFilter(findDirectChildren func(string) []string) *PathsFilter {
	return &PathsFilter{
		findDirectChildren: findDirectChildren,
		filteredPaths:      make(map[string]bool),
	}
}

func NewPathsFilterEmpty() *PathsFilter {
	return NewPathsFilter(func(string) []string {
		return nil
	})
}

func (p *PathsFilter) Clear() {
	p.filteredPaths = make(map[string]bool)
}

func (p *PathsFilter) Copy() PathsFilter {
	filteredPaths := make(map[string]bool, len(p.filteredPaths))
	for path, hidden := range p.filteredPaths {
		filteredPaths[path] = hidden
	}
	return PathsFilter{
		p.findDirectChildren,
		filteredPaths,
	}
}

func (p *PathsFilter) IsHiddenPath(path string) bool {
	if len(p.filteredPaths) == 0 {
		return false
	}
	// Types created since bulk hiding inherit their nearest visibility rule.
	// An explicit visible child takes precedence over a hidden ancestor.
	for {
		if hidden, ok := p.filteredPaths[path]; ok {
			return hidden
		}
		separator := strings.LastIndexByte(path, '/')
		if separator <= 0 {
			return false
		}
		path = path[:separator]
	}
}

func (p *PathsFilter) IsVisiblePath(path string) bool {
	return !p.IsHiddenPath(path)
}

func (p *PathsFilter) HasHiddenChildPath(path string) bool {
	for filteredPath, hidden := range p.filteredPaths {
		if hidden && (filteredPath == path || strings.HasPrefix(filteredPath, path+"/")) {
			return true
		}
	}
	return false
}

func (p *PathsFilter) TogglePath(path string) {
	p.SetVisible(path, p.IsHiddenPath(path))
	log.Printf("toggle [%s] path: [%t]", path, p.IsVisiblePath(path))
}

// SetVisible applies to the entire subtree, replacing previous exceptions.
func (p *PathsFilter) SetVisible(path string, visible bool) {
	for filteredPath := range p.filteredPaths {
		if filteredPath == path || strings.HasPrefix(filteredPath, path+"/") {
			delete(p.filteredPaths, filteredPath)
		}
	}
	if !visible {
		p.hidePath(path)
	} else if p.IsHiddenPath(path) {
		// Keep one exception at the subtree root, not one per descendant.
		p.filteredPaths[path] = false
	}
}

func (p *PathsFilter) hidePath(path string) {
	for _, directChild := range p.findDirectChildren(path) {
		p.hidePath(directChild)
	}
	p.filteredPaths[path] = true
}
