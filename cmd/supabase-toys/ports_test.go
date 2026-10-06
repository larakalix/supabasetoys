package main

import "testing"

func TestParsePortSettings(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values []string
		valid  bool
	}{
		{"valid", []string{"api.port=55000", "db.port=65535"}, true},
		{"zero", []string{"api.port=0"}, false},
		{"overflow", []string{"api.port=65536"}, false},
		{"fraction", []string{"api.port=3.5"}, false},
		{"missing key", []string{"=54321"}, false},
		{"missing separator", []string{"api.port"}, false},
		{"duplicate", []string{"api.port=55000", "api.port=55001"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePortSettings(tt.values)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v error=%v", tt.valid, err)
			}
		})
	}
}
