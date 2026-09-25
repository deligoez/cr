package sandbox

import (
	"slices"
	"strings"
)

// AgentVariables are the environment variables coding-agent detectors read,
// which §5.2.1 has cr withhold from the test runner.
//
// The list is the union of what vendor/laravel/agent-detector and
// vendor/ergebnis/agent-detector read, as found in tarfin-labs/backend's
// vendor directory. Measured on tarfin-labs/backend#6328 with cr 0.13.0: run
// under Claude Code (AI_AGENT=claude-code_…), Pest 4 printed
// `{"tool":"pest","result":"passed","tests":21,"passed":21,"assertions":154,"duration_ms":1571}`
// in place of its recap, so rounds 1 and 2 read tests_run null and passed
// false over an exit of 0, and round 3, with AI_AGENT unset, read 21 run, 0
// failed, and passed. A runner that detects an agent changes the very output
// tests.count_pattern reads.
var AgentVariables = []string{
	"AI_AGENT",
	"AMP_CURRENT_THREAD_ID",
	"ANTIGRAVITY_AGENT",
	"AUGMENT_AGENT",
	"CLAUDECODE",
	"CLAUDE_CODE",
	"CLAUDE_CODE_IS_COWORK",
	"CODEX_CI",
	"CODEX_SANDBOX",
	"CODEX_THREAD_ID",
	"COPILOT_ALLOW_ALL",
	"COPILOT_CLI",
	"COPILOT_GITHUB_TOKEN",
	"COPILOT_MODEL",
	"CURSOR_AGENT",
	"CURSOR_EXTENSION_HOST_ROLE",
	"CURSOR_TRACE_ID",
	"GEMINI_CLI",
	"KIRO_AGENT_PATH",
	"OPENCODE",
	"OPENCODE_CLIENT",
	"PI_CODING_AGENT",
	"REPL_ID",
}

// withoutAgent is environ without every entry AgentVariables names.
func withoutAgent(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if slices.Contains(AgentVariables, name) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
