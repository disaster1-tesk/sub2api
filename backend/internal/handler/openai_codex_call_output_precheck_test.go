package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasUnicodeEscapeSpelling(t *testing.T) {
	const needle = "function_call_output"
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"no escape", `{"type":"function_call_output"}`, false},
		{"terminal escape", `{"output":"\u001b[31mred\u001b[0m"}`, false},
		{"html-safe escapes", `{"output":"\u003cdiv\u003e \u0026"}`, false},
		{"non-ascii and surrogate pair", `{"output":"\u4e2d\u6587 \ud83d\ude00"}`, false},
		{"ascii not in any needle", `{"output":"\u0041\u007a"}`, false},
		{"lowercase hex spells needle char", `{"type":"\u0066unction_call_output"}`, true},
		{"uppercase hex spells needle char", `{"type":"function_call\u005Foutput"}`, true},
		{"spells tool name char only", `{"name":"\u0064o_it"}`, true},
		{"escaped backslash stays conservative", `{"output":"\\u0066"}`, true},
		{"malformed hex", `{"output":"\uZZZZ"}`, true},
		{"truncated escape", `{"output":"\u00`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, hasUnicodeEscapeSpelling([]byte(tc.body), needle, "do_it"))
		})
	}
}

// Escapes that cannot spell the type or a tool name are common in real bodies
// (terminal output, HTML-safe encoders) and must not force the full scan.
func TestMayContainCodexCallOutputCandidateIgnoresUnrelatedEscapes(t *testing.T) {
	unrelated := []byte(`{"input":[{"type":"message","content":"\u001b[0m \u003cb\u003e \u4e2d"}]}`)
	require.False(t, mayContainCodexCallOutputCandidate(unrelated, "automation_update"))

	literal := []byte(`{"input":[{"type":"function_call_output","name":"automation_update","output":"\u001b[0m"}]}`)
	require.True(t, mayContainCodexCallOutputCandidate(literal, "automation_update"))

	hiddenType := []byte(`{"input":[{"type":"function\u005fcall_output","name":"automation_update"}]}`)
	require.True(t, mayContainCodexCallOutputCandidate(hiddenType, "automation_update"))

	hiddenName := []byte(`{"input":[{"type":"function_call_output","name":"\u0061utomation_update"}]}`)
	require.True(t, mayContainCodexCallOutputCandidate(hiddenName, "automation_update"))
}
