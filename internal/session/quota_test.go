package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserAgent(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	tests := []struct {
		name string
		set  string
		want string
	}{
		{
			name: "unset build keeps a well-formed default",
			set:  "",
			want: "csm/dev (+https://github.com/yepzdk/claude-sessions-monitor)",
		},
		{
			// git describe hands main a leading "v"; the header carries the
			// bare version, matching the Makefile's PKG_VERSION.
			name: "release tag drops the leading v",
			set:  "v0.4.0",
			want: "csm/0.4.0 (+https://github.com/yepzdk/claude-sessions-monitor)",
		},
		{
			name: "untagged build passes through as-is",
			set:  "v0.4.0-3-gabc1234-dirty",
			want: "csm/0.4.0-3-gabc1234-dirty (+https://github.com/yepzdk/claude-sessions-monitor)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version = original
			if tt.set != "" {
				SetVersion(tt.set)
			}
			if got := userAgent(); got != tt.want {
				t.Errorf("userAgent() = %q, want %q", got, tt.want)
			}
			if strings.Contains(userAgent(), "claude-cli") {
				t.Error("userAgent() must not imitate another client")
			}
		})
	}
}

// A corrupt or truncated line can carry a digit run past int range. Unchecked
// accumulation wrapped negative and pulled the 5-hour aggregate down.
func TestExtractIntFieldRejectsOverflow(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{"normal", `{"usage":{"input_tokens":1234}}`, 1234},
		{"zero", `{"usage":{"input_tokens":0}}`, 0},
		{"absent", `{"usage":{"output_tokens":5}}`, 0},
		// Past int64. Unchecked accumulation wrapped this to a negative and
		// pulled the aggregate down.
		{"beyond int range", `{"usage":{"input_tokens":9223372036854775999}}`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIntField(tt.line, `"input_tokens":`)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
			if got < 0 {
				t.Errorf("negative token count %d would reduce the quota total", got)
			}
		})
	}
}

// bufio.Scanner stops silently on a line past its size cap. Returning the
// partial sum as if the scan completed understates the user's quota with no
// signal at all.
func TestScanLogTokensReportsTruncation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "big.jsonl")

	ts := time.Now().Format(time.RFC3339Nano)
	var b strings.Builder
	b.WriteString(`{"timestamp":"` + ts + `","usage":{"input_tokens":100,"output_tokens":0}}` + "\n")
	// One line past the 10MB scanner cap.
	b.WriteString(`{"timestamp":"` + ts + `","pad":"` + strings.Repeat("x", 11*1024*1024) + `"}` + "\n")
	b.WriteString(`{"timestamp":"` + ts + `","usage":{"input_tokens":900,"output_tokens":0}}` + "\n")

	if err := os.WriteFile(logFile, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	input, _, _, _, err := scanLogTokens(logFile, time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("scan stopped early but reported success; input=%d "+
			"(the 900 after the oversized line was dropped silently)", input)
	}
	if !strings.Contains(err.Error(), logFile) {
		t.Errorf("error does not name the file: %v", err)
	}
}

func TestComputeUsageReportsDiscoveryFailure(t *testing.T) {
	// HOME points nowhere, so DiscoverHistory cannot succeed.
	t.Setenv("HOME", filepath.Join(t.TempDir(), "does-not-exist"))
	u := ComputeUsage()
	if u == nil {
		t.Fatal("ComputeUsage returned nil")
	}
	if u.TotalTokens != 0 {
		t.Fatalf("unexpected tokens: %d", u.TotalTokens)
	}
	if u.Err == "" {
		t.Error("discovery failed but Err is empty; the UI renders this as " +
			`"No token usage in the past 5 hours."`)
	}
}

// The body of a failed usage-API response is read before the status is checked,
// and then used to be thrown away. "401 Unauthorized" is the same text whether
// the token expired, was revoked or never had the scope; the API says which, and
// the panel was reporting the status line instead of the answer.
func TestAPIErrorMessageCarriesTheAPIsOwnExplanation(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status string
		want   string
	}{
		{
			name:   "the expiry the endpoint actually reports",
			body:   `{"type":"error","error":{"type":"authentication_error","message":"OAuth access token has expired. Re-authenticate to continue."}}`,
			status: "401 Unauthorized",
			want:   "HTTP 401 Unauthorized: OAuth access token has expired. Re-authenticate to continue.",
		},
		{
			// A gateway or proxy failure, where the status line really is all
			// there is. Inventing a message here would be worse than the status.
			name:   "an HTML error page falls back to the status",
			body:   `<html><body>502 Bad Gateway</body></html>`,
			status: "502 Bad Gateway",
			want:   "HTTP 502 Bad Gateway",
		},
		{
			name:   "JSON with no message falls back to the status",
			body:   `{"type":"error","error":{"type":"overloaded_error"}}`,
			status: "529 ",
			want:   "HTTP 529 ",
		},
		{
			name:   "an empty body falls back to the status",
			body:   "",
			status: "500 Internal Server Error",
			want:   "HTTP 500 Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiErrorMessage([]byte(tt.body), tt.status); got != tt.want {
				t.Errorf("apiErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The dashboard branches on the reason, not the wording. A rate-limited endpoint
// wants to be left alone; a rejected credential wants a sign-in; the two must not
// arrive as the same cause.
func TestReasonForStatusSeparatesTheCausesTheDashboardActsOn(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{401, reasonUnauthorized},
		{403, reasonUnauthorized},
		{429, reasonRateLimited},
		{500, reasonAPIError},
		{502, reasonAPIError},
	}

	for _, tt := range tests {
		if got := reasonForStatus(tt.code); got != tt.want {
			t.Errorf("reasonForStatus(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// A body that does not parse is not an absent quota: the panel has to be able to
// tell "csm could not read the answer" from "there is no credential".
func TestParseAPIQuotaResponseReportsAParseFailureAsItsOwnCause(t *testing.T) {
	quota := parseAPIQuotaResponse([]byte(`{"five_hour":`))
	if quota.Available {
		t.Fatal("an unparseable response reported as available")
	}
	if quota.Reason != reasonParse {
		t.Errorf("Reason = %q, want %q", quota.Reason, reasonParse)
	}
}

// Every bucket the API sends must reach the panel with its own utilization and
// reset time. Sharing one decode step across the four is what keeps them from
// drifting, so this checks all four rather than a sample, and checks that a
// bucket the response omits stays nil instead of showing as 0%.
func TestParseAPIQuotaResponseFillsEveryBucketItWasSent(t *testing.T) {
	quota := parseAPIQuotaResponse([]byte(`{
		"five_hour":        {"utilization": 12.5, "resets_at": "2026-09-05T10:00:00Z"},
		"seven_day":        {"utilization": 40,   "resets_at": "2026-09-11T10:00:00Z"},
		"seven_day_sonnet": {"utilization": 55,   "resets_at": "2026-09-12T10:00:00Z"},
		"extra_usage":      {"is_enabled": true}
	}`))

	if !quota.Available {
		t.Fatalf("a well-formed response reported as unavailable: %q", quota.Error)
	}

	for _, tc := range []struct {
		name string
		got  *QuotaBucket
		util float64
		when string
	}{
		{"five_hour", quota.FiveHour, 12.5, "2026-09-05T10:00:00Z"},
		{"seven_day", quota.SevenDay, 40, "2026-09-11T10:00:00Z"},
		{"seven_day_sonnet", quota.SevenDaySonnet, 55, "2026-09-12T10:00:00Z"},
	} {
		if tc.got == nil {
			t.Errorf("%s missing; the panel would show no bar for a quota the API reported", tc.name)
			continue
		}
		if tc.got.Utilization != tc.util {
			t.Errorf("%s utilization = %v, want %v", tc.name, tc.got.Utilization, tc.util)
		}
		if tc.got.ResetsAt == nil {
			t.Errorf("%s reset time dropped; the panel would not say when the limit lifts", tc.name)
			continue
		}
		if got := tc.got.ResetsAt.UTC().Format(time.RFC3339); got != tc.when {
			t.Errorf("%s resets at %s, want %s", tc.name, got, tc.when)
		}
	}

	if quota.SevenDayOpus != nil {
		t.Error("a bucket the API did not send came back non-nil; the panel would draw it as 0% used")
	}
	if quota.ExtraUsage == nil || !quota.ExtraUsage.IsEnabled {
		t.Error("extra usage flag lost")
	}
}

// A reset time in a form time.Parse rejects must not cost the utilization too.
func TestParseAPIQuotaResponseKeepsUtilizationWhenTheResetTimeIsUnreadable(t *testing.T) {
	quota := parseAPIQuotaResponse([]byte(`{"five_hour":{"utilization":73,"resets_at":"tomorrow"}}`))
	if quota.FiveHour == nil {
		t.Fatal("bucket dropped over an unreadable reset time; the panel would show no usage at all")
	}
	if quota.FiveHour.Utilization != 73 {
		t.Errorf("Utilization = %v, want 73", quota.FiveHour.Utilization)
	}
	if quota.FiveHour.ResetsAt != nil {
		t.Error("an unparseable reset time came back as a real time")
	}
}
