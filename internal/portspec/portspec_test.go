package portspec

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParse_ValidSpecs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []uint16
	}{
		{"no arguments", nil, nil},
		{"single port", []string{"3000"}, []uint16{3000}},
		{"several ports sorted", []string{"5432", "3000"}, []uint16{3000, 5432}},
		{"range", []string{"3000-3002"}, []uint16{3000, 3001, 3002}},
		{"single-element range", []string{"3000-3000"}, []uint16{3000}},
		{"duplicate ports", []string{"3000", "3000"}, []uint16{3000}},
		{"overlapping range and port", []string{"3000-3002", "3001", "80"}, []uint16{80, 3000, 3001, 3002}},
		{"lowest and highest port", []string{"1", "65535"}, []uint16{1, 65535}},
		{"surrounding whitespace", []string{" 3000 "}, []uint16{3000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.args, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParse_InvalidSpecs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		bad  string
	}{
		{"zero", []string{"0"}, "0"},
		{"above maximum", []string{"65536"}, "65536"},
		{"not a number", []string{"abc"}, "abc"},
		{"reversed range", []string{"3010-3000"}, "3010-3000"},
		{"empty string", []string{""}, `""`},
		{"negative number", []string{"-5"}, "-5"},
		{"too many dashes", []string{"1-2-3"}, "1-2-3"},
		{"range starting at zero", []string{"0-10"}, "0-10"},
		{"range above maximum", []string{"65000-70000"}, "65000-70000"},
		{"valid followed by invalid", []string{"3000", "x"}, "x"},
		{"open range", []string{"3000-"}, "3000-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if err == nil {
				t.Fatalf("Parse(%q) = %v, want error", tt.args, got)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v does not wrap ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tt.bad) {
				t.Errorf("error %q does not name the bad argument %s", err, tt.bad)
			}
		})
	}
}
