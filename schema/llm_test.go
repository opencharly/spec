package schema_test

import (
	"strings"
	"testing"
)

// llm_test.go — the acceptance tests for the shared OpenAI-compatible LLM
// vocabulary (schema/llm.cue). They reuse unifyDef / unifyDefFinal from
// box_init_test.go (R3, one helper set), following the desktop_test.go shape:
// CORPUS cases use unifyDef (structural), TEETH use unifyDefFinal (required-field
// and closedness violations only surface under FINAL validation).

// CORPUS: a realistic #LLMSpec exactly as an authored consumer block reads — the
// local ollama default endpoint with an env-referenced key, a full sampling +
// structured-output parameter block, and reasoning control.
func TestLLMSpec_OllamaCorpus(t *testing.T) {
	if err := unifyDef(t, "#LLMSpec", `{
		base_url:      "http://localhost:11434/v1"
		model:         "deepseek-v4.1-flash:cloud"
		api_key:       "$env.OPENAI_API_KEY"
		idle_timeout:  "3m"
		max_retries:   2
		headers:       {"HTTP-Referer": "https://example.test"}
		params: {
			temperature:      0.2
			top_p:            0.9
			max_tokens:       4096
			seed:             7
			stop:             ["</s>"]
			reasoning_effort: "high"
			stream_options:   {include_usage: true}
			parallel_tool_calls: true
			tool_choice:      "auto"
			user:             "charly"
			metadata:         {lane: "eval"}
			extra:            {some_undocumented_knob: 7}
		}
	}`); err != nil {
		t.Fatalf("the ollama corpus block must unify against #LLMSpec: %v", err)
	}
}

// CORPUS: a vision-oriented spec (the consumer this def exists for) — a vision
// model plus a json_schema response contract.
func TestLLMSpec_VisionCorpus(t *testing.T) {
	if err := unifyDef(t, "#LLMSpec", `{
		model: "qwen3-vl:8b"
		params: {
			max_tokens: 300
			response_format: {
				type: "json_schema"
				json_schema: {
					name:   "verdict"
					schema: {type: "object"}
					strict: true
				}
			}
		}
	}`); err != nil {
		t.Fatalf("the vision corpus block must unify against #LLMSpec: %v", err)
	}
}

// CORPUS: an integer literal in a `number` param (temperature: 1) must be legal —
// the generator emits *float64 and CUE's `number` accepts an int literal.
func TestLLMParams_IntLiteralForNumber(t *testing.T) {
	if err := unifyDef(t, "#LLMParams", `{temperature: 1, top_p: 1}`); err != nil {
		t.Fatalf("an int literal must be legal for a number param: %v", err)
	}
}

// TEETH: every param def is CLOSED — an unknown field is rejected, naming it.
func TestLLMParams_ClosedRejectsUnknownField(t *testing.T) {
	err := unifyDefFinal(t, "#LLMParams", `{temperature: 0.2, bogus_knob: 1}`)
	if err == nil {
		t.Fatal("an unknown param field must be rejected (the defs are closed)")
	}
	if !strings.Contains(err.Error(), "bogus_knob") {
		t.Fatalf("the rejection must NAME the offending field, got: %v", err)
	}
}

// TEETH: #LLMSpec is closed too (a typo'd connection knob is not a silent drop).
func TestLLMSpec_ClosedRejectsUnknownField(t *testing.T) {
	err := unifyDefFinal(t, "#LLMSpec", `{baseurl: "http://x"}`)
	if err == nil {
		t.Fatal("a mistyped top-level field must be rejected")
	}
}

// TEETH: the numeric ranges hold (temperature is 0..2, penalties -2..2, top_p 0..1).
func TestLLMParams_Ranges(t *testing.T) {
	for _, bad := range []string{
		`{temperature: 3}`,
		`{top_p: 1.5}`,
		`{frequency_penalty: -3}`,
		`{presence_penalty: 2.5}`,
		`{max_tokens: 0}`,
	} {
		if err := unifyDefFinal(t, "#LLMParams", bad); err == nil {
			t.Errorf("%s must be rejected by the range constraint", bad)
		}
	}
}

// TEETH: the reasoning_effort enum is exactly the ollama-accepted set (ollama's
// own FromChatRequest validates high|medium|low|none; a wider enum like the
// OpenAI SDK's would pass here and fail at the server).
func TestLLMParams_ReasoningEffortEnum(t *testing.T) {
	if err := unifyDefFinal(t, "#LLMParams", `{reasoning_effort: "high"}`); err != nil {
		t.Fatalf("high must be accepted: %v", err)
	}
	if err := unifyDefFinal(t, "#LLMParams", `{reasoning_effort: "xhigh"}`); err == nil {
		t.Fatal("xhigh is not an ollama-accepted effort and must be rejected")
	}
}

// TEETH: response_format type is the three-value enum.
func TestLLMResponseFormat_TypeEnum(t *testing.T) {
	if err := unifyDefFinal(t, "#LLMResponseFormat", `{type: "json_object"}`); err != nil {
		t.Fatalf("json_object must be accepted: %v", err)
	}
	if err := unifyDefFinal(t, "#LLMResponseFormat", `{type: "xml"}`); err == nil {
		t.Fatal("an unknown response_format type must be rejected")
	}
}
