package schema

// CUE schema for the `builder` kind. #Builder validates ONE value of the
// `builder:` map (BuilderDef). CLOSED: every authored key is modeled (an unknown
// key is a typo). Template/script bodies are Go text/template (plain `string`).
// #CacheMount / #PhaseSet / #PhaseTemplates are shared (00_common.cue). No #Step
// (builder has no plan).

#Builder: {
	detect_file?: [...(string & !="")] @go(DetectFiles)
	requires_src_dir?: bool          @go(RequiresSrcDir)
	inline?:           bool
	cache_mount?: [...#CacheMount] @go(CacheMount)
	env?:              #StrMap
	runtime_env?:      #StrMap       @go(RuntimeEnv)
	manylinux_fix?:    string        @go(ManylinuxFix)
	build_script?:     string & !="" @go(BuildScript)
	phase?:            #PhaseSet     @go(Phases,optional=nillable)
	install_command?: {
		[string]: string
	} @go(InstallCommands)
	// There is no copy_artifact or copy_binary: what a builder copies into the target (its payload,
	// its tool binary) is the builder PLUGIN's own knowledge, returned in its #BuilderResolveReply,
	// never an authored field of this node (DESIGN.cue #BuilderAlt under D-PLUG-2; P3 derived, not
	// authored; opencharly/sdk#608).
	path_contribution?: [...(string & !="")] @go(PathContributions)
	kind:             *"layer" | "bootstrap"
	privileged:       *false | true
	output_artifact?: string & =~"^/" @go(OutputArtifact)
	if privileged {
		output_artifact!: string & =~"^/"
	}
}

// #BuilderPackage is what a builder declares it PRODUCED when its output is OS package files (aur
// builds `pac` packages): their format, the directory one candy's packages are in, and their glob.
// It is the builder PLUGIN's own knowledge, returned in its replies (#BuilderResolveReply,
// #BuilderCollectReply) — never authored on the builder node, whose DESIGN shape is a manager map
// (DESIGN.cue #BuilderAlt under D-PLUG-2; P3 derived not authored). A target whose distro serves that
// format's `local_pkg` contract installs the packages through it; every other target receives the
// builder's payload (opencharly/sdk#608).
#BuilderPackage: {
	format!: #BuildFormat
	dir!:    string & !=""
	glob!:   string & !=""
}

// #BuilderConfig is the `builder:` build-vocabulary container — the whole
// builder map keyed by builder name (charly/charly.yml's `builder:` section).
// #55 step 3-III: relocated from sdk/buildkit for the import-purity lever (see
// #DistroConfig). Its methods (ValidBuilderType / BuilderNames) are pure Go
// METHODS and stay hand-written in spec/builder_config_methods.go.
#BuilderConfig: {
	builder: {[string]: #Builder} @go(Builder,type=map[string]*Builder)
}
