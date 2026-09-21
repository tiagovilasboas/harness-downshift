// Package hookutil provides small utilities shared across harness adapters.
package hookutil

// StringField reads a string value from a decoded JSON object (map[string]any).
// Returns an empty string if the key is absent or the value is not a string.
func StringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
