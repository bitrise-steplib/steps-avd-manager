package avdconfig

import (
	"fmt"
	"strings"
)

// Properties ...
type Properties []Property

// ToSlice ...
func (properties Properties) ToSlice() []string {
	propertiesSlice := []string{}
	for _, property := range properties {
		propertiesSlice = append(propertiesSlice, property.String())
	}
	return propertiesSlice
}

// NewProperties ...
func NewProperties(content []string) (Properties, error) {
	props := Properties{}

	for _, line := range content {
		property, err := NewProperty(line)
		if err != nil {
			return nil, fmt.Errorf("error parsing properties, error: %s", err)
		}
		props.Apply(property.Key, property.Value)
	}

	return props, nil
}

func (properties Properties) String() string {
	return strings.Join(properties.ToSlice(), "\n")
}

// Apply ...
func (properties *Properties) Apply(key, value string) {
	property := Property{Key: key, Value: value}

	for i, line := range *properties {
		if line.Key == property.Key {
			if property.Value == "" {
				*properties = append((*properties)[:i], (*properties)[i+1:]...)
				return
			}
			(*properties)[i] = property
			return
		}
	}

	(*properties) = append((*properties), Property{Key: property.Key, Value: property.Value})
}

// Get ...
func (properties *Properties) Get(key string, defaultValue ...string) string {
	for _, line := range *properties {
		if line.Key == key {
			return line.Value
		}
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}

	return ""
}
