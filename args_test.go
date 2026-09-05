package main

import (
	"slices"
	"testing"

	"github.com/yepzdk/claude-sessions-monitor/internal/session"
)

// `csm upgrade` and `csm update` used to start the dashboard: flag stops parsing
// at the first non-flag argument, and nothing looked at what it left behind. The
// user who typed it saw a session list appear and had no way to tell that the
// word had been thrown away.
func TestResolveArgsAcceptsTheUpgradeSubcommand(t *testing.T) {
	for _, word := range []string{"upgrade", "update"} {
		upgrade, err := resolveArgs([]string{word})
		if err != nil {
			t.Errorf("resolveArgs([%q]) = %v, want an upgrade", word, err)
		}
		if !upgrade {
			t.Errorf("resolveArgs([%q]) did not ask for an upgrade, so csm would start the dashboard instead", word)
		}
	}
}

func TestResolveArgsLeavesNormalInvocationsAlone(t *testing.T) {
	upgrade, err := resolveArgs(nil)
	if err != nil || upgrade {
		t.Errorf("resolveArgs(nil) = (%v, %v), want (false, nil)", upgrade, err)
	}
}

// An argument csm does not understand has to be refused rather than dropped:
// ignoring it means running some other command than the one that was typed.
func TestResolveArgsRejectsWhatItCannotRun(t *testing.T) {
	for _, args := range [][]string{
		{"upgrades"},         // a near-miss, which must not silently start the dashboard
		{"--upgrade=please"}, // flag already refused it; it reaches here as a positional
		{"history"},
	} {
		if _, err := resolveArgs(args); err == nil {
			t.Errorf("resolveArgs(%q) was accepted, so csm would do something else instead", args)
		}
	}

	// Naming the extra argument matters: "upgrade takes no arguments" is
	// actionable where "unknown argument" would blame the wrong word.
	_, err := resolveArgs([]string{"upgrade", "now"})
	if err == nil {
		t.Fatal("resolveArgs([upgrade now]) was accepted")
	}
	if got := err.Error(); got != `upgrade takes no arguments, got "now"` {
		t.Errorf("error = %q, which does not name the argument that is wrong", got)
	}
}

// The harness filter (`f`, where the footer offers it) walks every agent and
// then returns to showing all of them. A cycle that skips an agent, or never
// comes back to "", leaves rows hidden with no key that brings them back.
//
// The agents are named here rather than read from session.Harnesses: deriving
// the expectation from the same slice the code walks would pass even if an
// agent were dropped from it.
func TestHarnessFilterCyclesEveryAgentAndBackToAll(t *testing.T) {
	var seen []session.Harness
	current := session.Harness("")
	for range len(session.Harnesses()) + 1 {
		current = nextHarnessFilter(current)
		seen = append(seen, current)
	}

	for _, want := range []session.Harness{session.HarnessClaude, session.HarnessOMP} {
		if !slices.Contains(seen, want) {
			t.Errorf("the %q filter is unreachable: pressing f never selects it, so its rows cannot be brought back", want)
		}
	}
	if last := seen[len(seen)-1]; last != "" {
		t.Errorf("cycle ended at %q, want \"\": the filter never returns to showing every agent", last)
	}
}
