package avdconfig

import (
	"strings"

	"github.com/bitrise-io/go-utils/fileutil"
)

// Model ...
type Model struct {
	filePath   string
	Properties *Properties
}

// New ...
func New(path string, initialProperties ...Property) *Model {
	properties := Properties(initialProperties)

	return &Model{
		filePath:   path,
		Properties: &properties,
	}
}

// Parse ...
func Parse(path string) (*Model, error) {
	content, err := fileutil.ReadStringFromFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(content, "\n")

	properties, err := NewProperties(lines)
	if err != nil {
		return nil, err
	}

	return &Model{filePath: path, Properties: &properties}, nil
}

// Save ...
func (model *Model) Save() error {
	return fileutil.WriteStringToFile(model.filePath, model.Properties.String())
}
