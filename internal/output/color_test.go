package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestUseColorDecision(t *testing.T) {
	// A buffer is never a terminal, which is the "piped output" case.
	piped := &bytes.Buffer{}

	tests := []struct {
		name string
		mode ColorMode
		env  map[string]string
		want bool
	}{
		{name: "piped output is not coloured", mode: ColorAuto, want: false},
		{name: "never means never", mode: ColorNever, want: false},
		{name: "always overrides the pipe", mode: ColorAlways, want: true},
		{
			name: "NO_COLOR wins over auto",
			mode: ColorAuto, env: map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, want: false,
		},
		{
			name: "NO_COLOR is honoured even when empty",
			mode: ColorAuto, env: map[string]string{"NO_COLOR": ""}, want: false,
		},
		{
			name: "CLICOLOR_FORCE colours a pipe",
			mode: ColorAuto, env: map[string]string{"CLICOLOR_FORCE": "1"}, want: true,
		},
		{
			name: "CLICOLOR_FORCE=0 does not",
			mode: ColorAuto, env: map[string]string{"CLICOLOR_FORCE": "0"}, want: false,
		},
		{
			name: "an explicit choice beats the environment",
			mode: ColorNever, env: map[string]string{"CLICOLOR_FORCE": "1"}, want: false,
		},
		{
			// A flag the user typed is a clearer signal than an environment
			// variable they may have inherited, so --color always wins.
			name: "asking for colour explicitly beats NO_COLOR",
			mode: ColorAlways, env: map[string]string{"NO_COLOR": "1"}, want: true,
		},
		{
			name: "dumb terminals get no escapes",
			mode: ColorAuto, env: map[string]string{"TERM": "dumb"}, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"NO_COLOR", "CLICOLOR_FORCE", "TERM"} {
				os.Unsetenv(key)
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if got := useColor(piped, tt.mode); got != tt.want {
				t.Errorf("useColor(%s) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestParseColorMode(t *testing.T) {
	for _, valid := range []string{"auto", "always", "never"} {
		if _, err := ParseColorMode(valid); err != nil {
			t.Errorf("ParseColorMode(%q) error = %v", valid, err)
		}
	}
	if _, err := ParseColorMode("yes"); err == nil {
		t.Error("expected an error for an unknown mode")
	} else if !strings.Contains(err.Error(), "auto, always or never") {
		t.Errorf("error should list the valid modes: %v", err)
	}
}

func TestValueColouring(t *testing.T) {
	coloured := Style{Color: true}
	plain := Style{Color: false}

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "a failing state is red", value: "CrashLoopBackOff", want: ansiRed},
		{name: "a working state is green", value: "Running", want: ansiGreen},
		{name: "a transitional state is yellow", value: "Pending", want: ansiYellow},
		{name: "nothing ready is red", value: "0 of 3", want: ansiRed},
		{name: "some ready is yellow", value: "2 of 3", want: ansiYellow},
		{name: "all ready is green", value: "3 of 3", want: ansiGreen},
		{name: "the slash form works too", value: "0/1 containers", want: ansiRed},
		{name: "a missing object is red", value: "does not exist", want: ansiRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coloured.value(tt.value)
			if !strings.Contains(got, tt.want) {
				t.Errorf("value(%q) = %q, want it to use %q", tt.value, got, tt.want)
			}
			if !strings.Contains(got, tt.value) {
				t.Errorf("value(%q) = %q, the text itself must survive", tt.value, got)
			}
			if plain.value(tt.value) != tt.value {
				t.Errorf("without colour the value must be untouched, got %q", plain.value(tt.value))
			}
		})
	}
}

// Words that only look like states must not be coloured, and a state inside a
// longer word is not that state.
func TestValueColouringIsConservative(t *testing.T) {
	coloured := Style{Color: true}
	for _, value := range []string{
		"256Mi", "registry.example.com/api:v2", "app=payments", "10.96.0.10",
		"NotReadyAddresses", "delay 5s, period 10s",
	} {
		if got := coloured.value(value); got != value {
			t.Errorf("value(%q) = %q, want it left alone", value, got)
		}
	}
}

// A note carries meaning when it names a state and recedes otherwise.
func TestNoteColouring(t *testing.T) {
	s := Style{Color: true}
	if got := s.note("Service not found"); !strings.Contains(got, ansiRed) {
		t.Errorf("note = %q, want the failure highlighted", got)
	}
	if got := s.note("from EndpointSlice"); !strings.Contains(got, ansiDim) {
		t.Errorf("note = %q, want plain notes dimmed", got)
	}
}
