// llm.cue — the SHARED OpenAI-compatible LLM vocabulary: the endpoint/connection
// config (#LLMSpec) and the general chat-completions request-parameter block
// (#LLMParams), plus the structured-output / reasoning / streaming / tool-choice
// companions.
//
// WHY IT LIVES IN spec (the contract module) rather than in one consumer plugin:
// it is a SHARED contract with TWO consumers — the generic pipeline engine's agent
// stages (candy/plugin-pipeline) and the `vision:` check verb (candy/plugin-vision,
// which validates a screenshot by sending it to the same endpoint). R3 (one
// canonical implementation per behavior) puts the vocabulary in ONE place; the
// boundary law puts authored CUE in this module. This is the SAME shape as
// schema/desktop.cue (plugin-facing vocabulary in spec, generating the Go types a
// plugin imports): a consumer plugin that must ALSO compile its schema STANDALONE
// carries a mirror + a parity test, exactly as plugin-desktop-kind and
// plugin-distro do.
//
// NO #SchemaVersion BUMP: these are purely additive optional defs with nothing to
// migrate — the same shape as the iso VmSource arm, #DiskLayout, and the desktop
// kinds (spec PR #74).
//
// #LLMSpec and #LLMParams are CLOSED: an unknown or wrongly-typed field is a LOAD
// error, never a silent drop. The `extra` escape hatch on #LLMParams is the ONE
// legal place for a genuinely undocumented request key.

// #LLMSpec is the endpoint + connection config for an OpenAI-compatible API.
//
// Resolution precedence is applied FIELD-WISE by the consumer (a lower layer fills
// only what the higher layers left unset):
//
//	env override > stage/step block > entity block > built-in default
//
// The built-in default is the LOCAL ollama server so a consumer needs no authored
// block to run. An empty api_key means ABSENT: the client sends NO Authorization
// header at all (the local ollama needs none) — a missing secret can never zero
// out another layer.
//
// api_key is REF-RESOLVED like every other authored string where the consumer
// supports refs, so the correct authoring is a reference
// (`api_key: $env.OPENAI_API_KEY` or a secret ref), never a literal committed key.
#LLMSpec: close({
	// base_url: the endpoint root INCLUDING the /v1 suffix
	// (e.g. http://localhost:11434/v1). The client appends /chat/completions.
	base_url?: string
	// model: the model identifier sent in the request (e.g. deepseek-v4.1-flash:cloud,
	// or a vision model such as qwen3-vl:8b for image input).
	model?: string
	// api_key: bearer credential; empty/absent => NO auth header is sent.
	api_key?: string
	// organization / project: the OpenAI-Organization / OpenAI-Project headers.
	organization?: string
	project?:      string
	// timeout: a Go duration bounding the WHOLE request (e.g. "10m"). Empty means
	// no whole-request deadline — the idle_timeout is the bound instead.
	timeout?: string
	// idle_timeout: a Go duration bounding the gap BETWEEN streaming chunks. This
	// is the primary liveness bound for a streaming consumer: a slow-but-
	// progressing generation is never cut off, while a silent provider fails in
	// bounded time.
	idle_timeout?: string
	// max_retries: automatic retries on a retryable HTTP status. Defaults to 2.
	max_retries?: int & >=0 @go(Max_retries,optional=nillable)
	// headers: extra request headers (e.g. an OpenRouter HTTP-Referer/X-Title).
	headers?: {[string]: string}
	// params: the general request parameters (see #LLMParams).
	params?: #LLMParams
})

// #LLMParams is the general OpenAI chat-completions request-parameter block.
// Field names match the wire API exactly. All fields are OPTIONAL: an omitted
// field is not sent at all (the server's own default applies), so a consumer never
// injects a value the author did not ask for.
#LLMParams: close({
	// temperature: sampling temperature (0..2).
	temperature?: number & >=0 & <=2 @go(Temperature,type=*float64)
	// top_p: nucleus sampling probability mass (0..1).
	top_p?: number & >=0 & <=1 @go(Top_p,type=*float64)
	// max_tokens: the completion token bound (ollama: num_predict).
	max_tokens?: int & >0 @go(Max_tokens,optional=nillable)
	// max_completion_tokens: the newer alias of max_tokens.
	max_completion_tokens?: int & >0 @go(Max_completion_tokens,optional=nillable)
	// frequency_penalty / presence_penalty: repetition controls (-2..2).
	frequency_penalty?: number & >=-2 & <=2 @go(Frequency_penalty,type=*float64)
	presence_penalty?:  number & >=-2 & <=2 @go(Presence_penalty,type=*float64)
	// seed: requests a reproducible generation where the server supports it.
	seed?: int @go(Seed,optional=nillable)
	// stop: one stop sequence, or a list of them.
	stop?: string | [...string]
	// response_format: the structured-output contract (text | json_object |
	// json_schema).
	response_format?: #LLMResponseFormat
	// reasoning_effort: thinking control for reasoning models ("none" disables
	// thinking where the server honours it).
	reasoning_effort?: "high" | "medium" | "low" | "none" @go(Reasoning_effort,type=string)
	// reasoning: the object form of the same control (ollama accepts either).
	reasoning?: #LLMReasoning
	// stream_options: streaming response options.
	stream_options?: #LLMStreamOptions
	// parallel_tool_calls: permit the model to emit several tool calls per turn.
	parallel_tool_calls?: bool @go(Parallel_tool_calls,optional=nillable)
	// tool_choice: "none" | "auto" | "required" | {function: {name}}.
	tool_choice?: "none" | "auto" | "required" | #LLMNamedToolChoice
	// logprobs / top_logprobs: token log-probability reporting (unsupported by
	// the local ollama OpenAI layer; authorable for a full OpenAI endpoint).
	logprobs?:     bool @go(Logprobs,optional=nillable)
	top_logprobs?: int  @go(Top_logprobs,optional=nillable)
	// user: an end-user identifier for abuse monitoring.
	user?: string
	// metadata: arbitrary string metadata attached to the request.
	metadata?: {[string]: string}
	// logit_bias: per-token-id bias map.
	logit_bias?: {[string]: int}
	// extra: undocumented request fields, merged into the request body verbatim as
	// dotted JSON paths (sjson). The ONE legal place for an unknown key.
	extra?: {[string]: _}
})

// #LLMResponseFormat — the structured-output contract. type "json_schema" requires
// the json_schema block; the schema field is the JSON Schema itself.
#LLMResponseFormat: close({
	type: "text" | "json_object" | "json_schema" @go(Type,type=string)
	json_schema?: close({
		name:         string
		description?: string
		schema:       {[string]: _}
		strict?:      bool
	})
})

// #LLMReasoning — the object form of the reasoning/thinking control.
#LLMReasoning: close({
	effort?: "high" | "medium" | "low" | "none" @go(Effort,type=string)
})

// #LLMStreamOptions — streaming response options.
#LLMStreamOptions: close({
	include_usage?: bool @go(Include_usage,optional=nillable)
})

// #LLMNamedToolChoice — force one named function tool.
#LLMNamedToolChoice: close({
	function: close({name: string})
})
