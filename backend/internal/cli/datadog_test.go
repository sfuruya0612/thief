package cli

import (
	"testing"
)

func TestIsValidDatadogView(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "summary", input: "summary", want: true},
		{name: "sub-org", input: "sub-org", want: true},
		{name: "empty", input: "", want: false},
		{name: "invalid", input: "detail", want: false},
		{name: "uppercase", input: "SUMMARY", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidDatadogView(tt.input); got != tt.want {
				t.Errorf("isValidDatadogView(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDatadogMonths(t *testing.T) {
	tests := []struct {
		name       string
		startMonth string
		endMonth   string
		wantStart  string
		wantEnd    string
		wantErr    bool
	}{
		{
			name:       "start only",
			startMonth: "2026-05",
			wantStart:  "2026-05",
			wantEnd:    "",
		},
		{
			name:       "end month is shifted to next month",
			startMonth: "2026-05",
			endMonth:   "2026-06",
			wantStart:  "2026-05",
			wantEnd:    "2026-07",
		},
		{
			name:       "end month at year boundary",
			startMonth: "2025-11",
			endMonth:   "2025-12",
			wantStart:  "2025-11",
			wantEnd:    "2026-01",
		},
		{
			name:    "start month required",
			wantErr: true,
		},
		{
			name:       "invalid start month",
			startMonth: "2026/05",
			wantErr:    true,
		},
		{
			name:       "invalid end month",
			startMonth: "2026-05",
			endMonth:   "last-month",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd, err := parseDatadogMonths(tt.startMonth, tt.endMonth)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDatadogMonths(%q, %q) err = %v, wantErr %v", tt.startMonth, tt.endMonth, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if gotStart != tt.wantStart || gotEnd != tt.wantEnd {
				t.Errorf("parseDatadogMonths(%q, %q) = (%q, %q), want (%q, %q)",
					tt.startMonth, tt.endMonth, gotStart, gotEnd, tt.wantStart, tt.wantEnd)
			}
		})
	}
}
