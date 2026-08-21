package goldendb

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadRegistry(path string) (*Registry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registry Registry
	if err := yaml.Unmarshal(content, &registry); err != nil {
		return nil, fmt.Errorf("parse registry failed: %w", err)
	}
	registry.buildIndexes()
	return &registry, nil
}

func (r *Registry) buildIndexes() {
	r.toolIndex = map[string]Tool{}
	r.asyncIndex = map[string]AsyncPair{}
	for _, group := range r.ToolGroups {
		for _, tool := range group.Tools {
			tool.Group = group.Group
			tool.Method = strings.ToUpper(strings.TrimSpace(tool.Method))
			r.toolIndex[tool.Name] = tool
		}
	}
	for _, pair := range r.AsyncPairs {
		r.asyncIndex[pair.Action] = pair
	}
}

func (r *Registry) FindTool(name string) (Tool, bool) {
	if r.toolIndex == nil {
		r.buildIndexes()
	}
	tool, ok := r.toolIndex[name]
	return tool, ok
}

func (r *Registry) FindAsyncPair(action string) (AsyncPair, bool) {
	if r.asyncIndex == nil {
		r.buildIndexes()
	}
	pair, ok := r.asyncIndex[action]
	return pair, ok
}
