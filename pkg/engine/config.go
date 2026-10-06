package engine

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Text         string
	Document     map[string]any
	ProjectID    string
	Ports        map[string]uint16
	Experimental bool
}
type portDefinition struct {
	table, key string
	fallback   uint16
	enabled    bool
}

var portCatalog = []portDefinition{
	{table: "api", key: "port", fallback: 54321, enabled: true},
	{table: "db", key: "port", fallback: 54322, enabled: true},
	{table: "db", key: "shadow_port", fallback: 54320, enabled: true},
	{table: "db.pooler", key: "port", fallback: 54329},
	{table: "studio", key: "port", fallback: 54323, enabled: true},
	{table: "inbucket", key: "port", fallback: 54324, enabled: true},
	{table: "inbucket", key: "smtp_port", fallback: 54325},
	{table: "inbucket", key: "pop3_port", fallback: 54326},
	{table: "local_smtp", key: "port", fallback: 54324, enabled: true},
	{table: "local_smtp", key: "smtp_port", fallback: 54325},
	{table: "local_smtp", key: "pop3_port", fallback: 54326},
	{table: "analytics", key: "port", fallback: 54327, enabled: true},
	{table: "analytics", key: "vector_port", fallback: 54328, enabled: true},
	{table: "edge_runtime", key: "inspector_port", fallback: 8083, enabled: true},
}
var identityPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func section(doc map[string]any, name string) map[string]any {
	value := doc
	for _, part := range strings.Split(name, ".") {
		next, ok := value[part].(map[string]any)
		if !ok {
			return nil
		}
		value = next
	}
	return value
}
func ParseConfig(text string) (Config, error) {
	doc := map[string]any{}
	if err := toml.Unmarshal([]byte(text), &doc); err != nil {
		return Config{}, fmt.Errorf("invalid supabase/config.toml; fix TOML before managing this project: %w", err)
	}
	id, _ := doc["project_id"].(string)
	if !identityPattern.MatchString(id) {
		return Config{}, errors.New("project_id must contain only letters, digits, underscores, and hyphens")
	}
	experimental, _ := section(doc, "experimental")["stack"].(bool)
	config := Config{Text: text, Document: doc, ProjectID: id, Ports: map[string]uint16{}, Experimental: experimental}
	for _, definition := range portCatalog {
		_, smtp := doc["local_smtp"]
		if definition.table == "inbucket" && smtp {
			continue
		}
		if definition.table == "local_smtp" && !smtp {
			continue
		}
		table := section(doc, definition.table)
		enabled := definition.enabled
		if value, ok := table["enabled"].(bool); ok {
			enabled = value
		}
		if !enabled && definition.table != "db" {
			continue
		}
		specified, exists := table[definition.key]
		if definition.key != "port" && definition.table != "db" && definition.table != "edge_runtime" && !exists {
			continue
		}
		if experimental && !exists {
			continue
		}
		port := int64(definition.fallback)
		if exists {
			value, ok := specified.(int64)
			if !ok {
				return Config{}, fmt.Errorf("%s.%s port must be an integer", definition.table, definition.key)
			}
			port = value
		}
		if port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("%s.%s must be between 1 and 65535", definition.table, definition.key)
		}
		config.Ports[definition.table+"."+definition.key] = uint16(port)
	}
	return config, nil
}
func PortAvailable(port uint16) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))), 50*time.Millisecond)
		if err == nil {
			connection.Close()
			return false
		}
	}
	for _, network := range []string{"tcp4", "tcp6"} {
		listener, err := net.Listen(network, net.JoinHostPort("", strconv.Itoa(int(port))))
		if err != nil {
			return false
		}
		if err := listener.Close(); err != nil {
			return false
		}
	}
	return true
}
func Allocate(config Config, reserved map[uint16]bool, available func(uint16) bool) ([]PortChange, error) {
	used := maps.Clone(reserved)
	for _, port := range config.Ports {
		used[port] = true
	}
	seen := map[uint16]bool{}
	changes := []PortChange{}
	for _, key := range slices.Sorted(maps.Keys(config.Ports)) {
		port := config.Ports[key]
		conflict := reserved[port] || !available(port) || seen[port]
		seen[port] = true
		if !conflict {
			continue
		}
		var next uint16
		for candidate := uint16(20000); candidate <= 32767; candidate++ {
			if !used[candidate] && available(candidate) {
				next = candidate
				break
			}
		}
		if next == 0 {
			return nil, errors.New("no free local port in allocation range 20000–32767")
		}
		used[next] = true
		changes = append(changes, PortChange{Key: key, Before: port, After: next})
	}
	return changes, nil
}

// Patch preserves source text. A semantic comparison refuses syntaxes that cannot
// be edited without altering unrelated settings (including multiline strings).
func Patch(config Config, updates map[string]any) (string, error) {
	newline := "\n"
	if strings.Contains(config.Text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(config.Text, "\r\n", "\n"), "\n")
	for _, path := range slices.Sorted(maps.Keys(updates)) {
		table, key := "", path
		if index := strings.LastIndex(path, "."); index >= 0 {
			table = path[:index]
			key = path[index+1:]
		}
		encoded := fmt.Sprint(updates[path])
		if value, ok := updates[path].(string); ok {
			encoded = strconv.Quote(value)
		}
		current := ""
		count := 0
		insertAt := -1
		pattern := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(key) + `\s*=\s*)([^#]*?)(\s*(?:#.*)?)$`)
		for index, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "[") {
				if current == table && insertAt < 0 {
					insertAt = index
				}
				current = strings.TrimSpace(strings.TrimSuffix(strings.SplitN(trim, "]", 2)[0], "]"))
				current = strings.TrimPrefix(current, "[")
			}
			if current != table {
				continue
			}
			parts := pattern.FindStringSubmatch(line)
			if parts == nil {
				continue
			}
			lines[index] = parts[1] + encoded + parts[3]
			count++
		}
		if count > 1 {
			return "", errors.New("configuration syntax is ambiguous; edit this setting manually")
		}
		if count == 0 {
			if current == table {
				insertAt = len(lines)
			}
			assignment := key + " = " + encoded
			if insertAt >= 0 {
				lines = slices.Insert(lines, insertAt, assignment)
			} else {
				lines = append(lines, "["+table+"]", assignment)
			}
		}
	}
	next := strings.Join(lines, newline)
	parsed := map[string]any{}
	if err := toml.Unmarshal([]byte(next), &parsed); err != nil {
		return "", fmt.Errorf("cannot safely preserve this TOML syntax; edit it manually: %w", err)
	}
	original := map[string]any{}
	if err := toml.Unmarshal([]byte(config.Text), &original); err != nil {
		return "", fmt.Errorf("read original TOML: %w", err)
	}
	for path, value := range updates {
		parts := strings.Split(path, ".")
		target := original
		for _, part := range parts[:len(parts)-1] {
			child, ok := target[part].(map[string]any)
			if !ok {
				child = map[string]any{}
				target[part] = child
			}
			target = child
		}
		target[parts[len(parts)-1]] = value
	}
	if !reflect.DeepEqual(original, parsed) {
		return "", errors.New("cannot safely edit this TOML syntax while preserving unrelated settings; edit it manually")
	}
	return next, nil
}
