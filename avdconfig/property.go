package avdconfig

import (
	"fmt"
	"strings"
)

// Property ...
type Property struct {
	Key   string
	Value string
}

// NewProperty ...
func NewProperty(property string) (Property, error) {
	property = strings.TrimSpace(property)

	split := strings.Split(property, "=")

	if len(split) < 2 {
		return Property{}, fmt.Errorf("failed to parse property: %s, error: invalid format - format should be key=value", property)
	}

	key := split[0]
	key = strings.TrimSpace(key)

	value := strings.Join(split[1:], "=")
	value = strings.TrimSpace(value)

	return Property{
		Key:   key,
		Value: value,
	}, nil
}

func (property Property) String() string {
	return fmt.Sprintf("%s=%s", property.Key, property.Value)
}
