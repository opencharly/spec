package schema

// Generic agent-control, target, and terminal contracts. These definitions own
// the JSON payloads carried inside protocol.ChannelFrame; protobuf owns only the
// transport envelope. Runtime names and transport combinations are data.

#UUIDv7: string & =~"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"

#TargetHop: {
	transport:      or(_targetHopTransportWord)
	address?:       string
	user?:          string
	port?:          int & >0 & <=65535
	identity_file?: string @go(IdentityFile)
	command?: [...string]
	env?: #StrMap
	// Transport-native argv options expressed as deterministic key/value
	// data. SSH renders these as repeated `-o key=value` arguments; no shell
	// interpolation is involved.
	options?: #StrMap
}

#TargetSpec: {
	// Ordered outer-to-inner route after placement. An empty route is the
	// selected process. deployment/instance first place that process in any
	// Charly deployment; the remaining route composes identically there.
	hops?: [...#TargetHop]
	deployment?:  string
	instance?:    string
	working_dir?: string @go(WorkingDir)
}

#ProcessLaunch: {
	argv: [string & !="", ...string]
	working_dir?: string @go(WorkingDir)
	env?: #StrMap
}

#AgentRuntimeCapability: {
	provider:         string & !=""
	protocol_version: int & >=1 @go(ProtocolVersion,type=int)
	entrypoint?: [...string]
	profiles?: [...string]
	interaction_modes?: [...or(_interactionModesWord)] @go(InteractionModes)
	persistence?:         or(_agentRuntimeCapabilityPersistenceWord)
	semantic_completion?: bool @go(SemanticCompletion)
}

#AgentSession: {
	id:           #UUIDv7 @go(ID)
	runtime:      string & !=""
	target:       #TargetSpec
	state:        or(_agentSessionStateWord)
	created_at:   string @go(CreatedAt)
	updated_at:   string @go(UpdatedAt)
	storage_ref?: string @go(StorageRef)
	metadata?:    #StrMap
	terminal_profile?: #TerminalProfile @go(TerminalProfile,optional=nillable)
}

#AgentRunRequest: {
	id:              #UUIDv7       @go(ID)
	session_id:      #UUIDv7       @go(SessionID)
	request_id:      #UUIDv7       @go(RequestID)
	idempotency_key: string & !="" @go(IdempotencyKey)
	prompt?:         string
	params?: {...} @go(Params,type=map[string]any)
	resume?: bool
}

// Provider-reported durable session correlation. Runtime-specific session
// state remains on the target; this is the only binding replicated to the
// controller.
#AgentSessionBinding: {
	storage_ref: string & !="" @go(StorageRef)
}

#AgentAbortControl: {
	run_id:       #UUIDv7 @go(RunID)
	request_id:   #UUIDv7 @go(RequestID)
	requested_at: string  @go(RequestedAt)
}

#AgentEvent: {
	run_id:   #UUIDv7 @go(RunID)
	sequence: int & >=1
	type:     or(_agentEventTypeWord)
	time:     string
	payload?: {...} @go(Payload,type=map[string]any)
}

#AgentTeamMember: {
	name:    string & !=""
	runtime: string & !=""
	role?:   string
	target?: #TargetSpec
	terminal_profile?: #TerminalProfile @go(TerminalProfile,optional=nillable)
}

#AgentDelegationEdge: {
	from: string & !=""
	to:   string & !=""
	allow?: [...string]
}

#AgentTeam: {
	description?: string & !=""
	agents: [#AgentTeamMember, ...#AgentTeamMember]
	edges?: [...#AgentDelegationEdge]
	coordinator?:     string
	concurrency?:     int & >=1
	evidence_policy?: or(_evidencePolicyWord) @go(EvidencePolicy)
}

// Durable control-plane projection of a declarative team. The authored graph
// remains #AgentTeam; this record binds each member name to the ephemeral
// session created for one dispatch so later delegation can re-check authority.
#AgentTeamRecord: {
	id:         #UUIDv7 @go(ID)
	team:       #AgentTeam
	sessions:   {[string]: #UUIDv7}
	created_at: string @go(CreatedAt)
}

#AgentFederationRecord: {
	id:         #UUIDv7 @go(ID)
	node:       string & !=""
	owner:     string & !=""
	session_id?: #UUIDv7 @go(SessionID)
	run_id?:     #UUIDv7 @go(RunID)
	state:      or(_agentFederationRecordStateWord)
	updated_at: string @go(UpdatedAt)
	metadata?: #StrMap
}

#Incident: {
	id:         #UUIDv7 @go(ID)
	run_id?:    #UUIDv7 @go(RunID)
	state:      or(_incidentStateWord)
	summary:    string & !=""
	created_at: string @go(CreatedAt)
	evidence_refs?: [...string] @go(EvidenceRefs)
}

#RCARecord: {
	id:          #UUIDv7 @go(ID)
	incident_id: #UUIDv7 @go(IncidentID)
	state:       or(_rcaRecordStateWord)
	findings?: [...string]
	root_cause?:   string @go(RootCause)
	completed_at?: string @go(CompletedAt)
}

#RecoveryParams: {
	run_id?:           #UUIDv7 @go(RunID)
	session_id?:       #UUIDv7 @go(SessionID)
	target?:           #TargetSpec @go(Target,type=*TargetSpec)
	terminal_profile?: #TerminalProfile @go(TerminalProfile,type=*TerminalProfile)
	provider?:         string & !=""
	prompt?:           string
	runtime?:          string & !=""
	deployment?:       string & !=""
	note?:             string & !=""
}

#RecoveryDecision: {
	id:                          #UUIDv7 @go(ID)
	incident_id:                 #UUIDv7 @go(IncidentID)
	rca_id?:                     #UUIDv7 @go(RCAID)
	action:                      or(_recoveryDecisionActionWord)
	authorized_emergency_abort?: bool @go(AuthorizedEmergencyAbort)
	params?:                     #RecoveryParams @go(Params,type=*RecoveryParams)
	state:                       or(_recoveryDecisionStateWord)
	decided_at:                  string @go(DecidedAt)
	applied_at?:                 string @go(AppliedAt)
	error?:                      string
}

#TerminalProfile: {
	name: string & !=""
	entrypoint: [string, ...string]
	working_dir?: string @go(WorkingDir)
	env?:         #StrMap
	cols:         *120 | (int & >0 & <=1000)
	rows:         *40 | (int & >0 & <=1000)
	readiness?: {...} @go(Readiness,type=map[string]any)
	semantic_adapter?: string @go(SemanticAdapter)
	keys?: [...string]
	signals?: [...string]
	persistence?: or(_terminalProfilePersistenceWord)
	transcript?:  or(_transcriptWord)
}

#TerminalInput: {
	kind:    or(_terminalInputKindWord)
	text?:   string
	key?:    string
	cols?:   int & >0 & <=1000
	rows?:   int & >0 & <=1000
	signal?: string
}

#TerminalKey: {
	key: string & !=""
}

#TerminalResize: {
	cols: int & >0 & <=1000
	rows: int & >0 & <=1000
}

#TerminalFrame: {
	run_id:     #UUIDv7 @go(RunID)
	sequence:   int & >=1
	kind:       or(_terminalFrameKindWord)
	stream?:    or(_streamWord)
	data?:      bytes
	snapshot?:  string
	status?:    string
	exit_code?: int @go(ExitCode,type=*int)
	error?:     string
}

#TerminalSnapshot: {
	run_id:      #UUIDv7 @go(RunID)
	sequence:    int & >=0
	cols:        int & >0
	rows:        int & >0
	screen:      string
	cursor_col?: int & >=0 @go(CursorCol)
	cursor_row?: int & >=0 @go(CursorRow)
}

#TerminalExit: {
	run_id:    #UUIDv7 @go(RunID)
	exit_code: int     @go(ExitCode)
	reason?:   string
	time:      string
}

// --- vocabulary tables (S3) --------------------------------------------------
// Every closed set of words is a table keyed by word, `<x>Word` is derived from it,
// and the enum an author meets is `or(<x>Word)` — so the table and the enum cannot
// disagree and a new word is ONE row here. `@vocab` names the owner (§3.1): these
// sets are compiled into THIS contract module, so the owner is `spec`.
//
// Each table is named after its word when that name is unique in the schema, and
// `<definition><Field>` when it is not; the comment above it names its site.

// #TargetHop.transport
_targetHopTransport: {inproc: {}, exec: {}, ssh: {}, grpc: {}, tmux: {}} @vocab(spec)
_targetHopTransportWord: [for w, _ in _targetHopTransport {w}]
// #AgentRuntimeCapability.interaction_modes
_interactionModes: {structured: {}, terminal: {}} @vocab(spec)
_interactionModesWord: [for w, _ in _interactionModes {w}]
// #AgentRuntimeCapability.persistence
_agentRuntimeCapabilityPersistence: {none: {}, session: {}, required: {}} @vocab(spec)
_agentRuntimeCapabilityPersistenceWord: [for w, _ in _agentRuntimeCapabilityPersistence {w}]
// #AgentSession.state
_agentSessionState: {new: {}, active: {}, detached: {}, closed: {}, failed: {}} @vocab(spec)
_agentSessionStateWord: [for w, _ in _agentSessionState {w}]
// #AgentEvent.type
_agentEventType: {
	started: {}
	message: {}
	tool: {}
	delegation: {}
	settled: {}
	status: {}
	completed: {}
	aborted: {}
	failed: {}
} @vocab(spec)
_agentEventTypeWord: [for w, _ in _agentEventType {w}]
// #AgentTeam.evidence_policy
_evidencePolicy: {target: {}, coordinator: {}, both: {}} @vocab(spec)
_evidencePolicyWord: [for w, _ in _evidencePolicy {w}]
// #AgentFederationRecord.state
_agentFederationRecordState: {delegated: {}, active: {}, settled: {}, failed: {}} @vocab(spec)
_agentFederationRecordStateWord: [for w, _ in _agentFederationRecordState {w}]
// #Incident.state
_incidentState: {
	needs_rca: {}
	rca_active: {}
	awaiting_recovery: {}
	resolved: {}
} @vocab(spec)
_incidentStateWord: [for w, _ in _incidentState {w}]
// #RCARecord.state
_rcaRecordState: {active: {}, complete: {}} @vocab(spec)
_rcaRecordStateWord: [for w, _ in _rcaRecordState {w}]
// #RecoveryDecision.action
_recoveryDecisionAction: {
	reattach: {}
	resume: {}
	restart: {}
	"rebuild-target": {}
	"change-runtime": {}
	reassign: {}
	abort: {}
	operator: {}
} @vocab(spec)
_recoveryDecisionActionWord: [for w, _ in _recoveryDecisionAction {w}]
// #RecoveryDecision.state
_recoveryDecisionState: {planned: {}, applied: {}, failed: {}} @vocab(spec)
_recoveryDecisionStateWord: [for w, _ in _recoveryDecisionState {w}]
// #TerminalProfile.persistence
_terminalProfilePersistence: {none: {}, detach: {}, required: {}} @vocab(spec)
_terminalProfilePersistenceWord: [for w, _ in _terminalProfilePersistence {w}]
// #TerminalProfile.transcript
_transcript: {none: {}, raw: {}, screen: {}, both: {}} @vocab(spec)
_transcriptWord: [for w, _ in _transcript {w}]
// #TerminalInput.kind
_terminalInputKind: {
	text: {}
	paste: {}
	command: {}
	key: {}
	resize: {}
	signal: {}
	"close": {}
} @vocab(spec)
_terminalInputKindWord: [for w, _ in _terminalInputKind {w}]
// #TerminalFrame.kind
_terminalFrameKind: {
	raw: {}
	screen: {}
	status: {}
	exit: {}
	error: {}
	resync: {}
} @vocab(spec)
_terminalFrameKindWord: [for w, _ in _terminalFrameKind {w}]
// #TerminalFrame.stream
_stream: {stdout: {}, stderr: {}, terminal: {}} @vocab(spec)
_streamWord: [for w, _ in _stream {w}]
