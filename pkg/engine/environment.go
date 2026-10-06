package engine

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func MappedValues(values, mapping map[string]string) (map[string]string, error) {
	updates := map[string]string{}
	for source, target := range mapping {
		if !variablePattern.MatchString(target) {
			return nil, fmt.Errorf("invalid environment variable name: %s", target)
		}
		if Privileged(source) && PublicVariable(target) {
			return nil, fmt.Errorf("privileged %s cannot be exported to browser-exposed %s", source, target)
		}
		value, exists := values[source]
		if !exists {
			return nil, fmt.Errorf("runtime status does not contain %s", source)
		}
		if PublicVariable(target) && PrivilegedValue(value) {
			return nil, fmt.Errorf("privileged runtime value cannot be exported to browser-exposed %s", target)
		}
		if strings.ContainsAny(value, "\n\r\x00") {
			return nil, errors.New("invalid multiline environment value")
		}
		if _, exists := updates[target]; exists {
			return nil, fmt.Errorf("two source values map to %s", target)
		}
		updates[target] = value
	}
	return updates, nil
}
func MergeEnv(existing string, updates map[string]string) (string, error) {
	newline := "\n"
	if strings.Contains(existing, "\r\n") {
		newline = "\r\n"
	}
	seen := map[string]bool{}
	lines := []string{}
	normalized := strings.ReplaceAll(existing, "\r\n", "\n")
	for _, line := range strings.Split(strings.TrimSuffix(normalized, "\n"), "\n") {
		if existing == "" {
			break
		}
		trimmed := strings.TrimLeft(line, " \t")
		assignment := strings.TrimPrefix(trimmed, "export ")
		key, _, _ := strings.Cut(assignment, "=")
		key = strings.TrimSpace(key)
		if value, exists := updates[key]; exists {
			if seen[key] {
				return "", fmt.Errorf("environment file has duplicate %s; resolve it before writing", key)
			}
			seen[key] = true
			prefix := ""
			if strings.HasPrefix(trimmed, "export ") {
				prefix = "export "
			}
			line = prefix + key + "=" + encodeEnv(value)
		}
		lines = append(lines, line)
	}
	for _, key := range slices.Sorted(maps.Keys(updates)) {
		if !seen[key] {
			lines = append(lines, key+"="+encodeEnv(updates[key]))
		}
	}
	return strings.Join(lines, newline) + newline, nil
}
func encodeEnv(value string) string {
	// Double quotes round-trip backslashes and quotes in conventional dotenv parsers.
	return strconv.Quote(value)
}
