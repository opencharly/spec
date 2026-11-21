package schema

// CUE schema for a KIND's owned content (M1, Content -> IR; ops.OpContent).
//
// A node of a plugin kind can be OWNED by a candy — a `skill:` node names its owner — and a candy's
// content is what it compiles to (DESIGN §1.1 M1, D-IR-1). The kind that defines the node is the one
// place that knows what its nodes contribute, so the build/deploy kit never interprets them: for one
// box it asks every kind that declares `contents` (the envelope's content_kinds) with an
// #OwnedContentRequest, and installs what the #OwnedContentReply says — the files staged into the
// image (one COPY onto the image user's home) or applied as FileSteps on a machine venue, and the
// OCI labels merged into the image's baked labels. The kit is kind-agnostic; the kind owns the rule.

// #OwnedContentRequest — what ONE box offers a kind: the image user, the composed candies (in chain
// order, each with the root of the repository that authors it, where a node's derived body lives),
// and the kind's authored nodes in the box's OWNING scope (an imported namespace's own document for a
// namespaced box). Ownership is the kind's to decide from `nodes` against `candies`.
#OwnedContentRequest: {
	// box — the box whose composed candies are asked about (a build or a deploy). ABSENT means
	// PROJECT scope: `candies` is every candy of the project, and the reply's `diagnostics` are what
	// `charly box validate` reports — the kind checks what its nodes would contribute anywhere.
	box?:  string & !="" @go(Box)
	// The image user the files install for — required exactly when a box is named (an install
	// needs a home; project-scope validation installs nothing).
	home?: string & =~"^/" @go(Home)
	uid?:  int @go(UID,type=int)
	gid?:  int @go(GID,type=int)
	if box != _|_ {
		home!: _
		uid!:  _
		gid!:  _
	}
	candies?: [...#OwnedContentCandy] @go(Candies)
	// images — the names that are IMAGES (a `candy:` node with `base:`/`from:`) in this scope. An
	// image has no source directory, so a node it owns has no derivable body: the kind diagnoses it.
	images?: [...string] @go(Images)
	nodes?: {[string]: _} @go(Nodes,type=map[string]RawBody)
}

// #OwnedContentCandy — one composed candy, by name, with the absolute root of the repository that
// authors it. A candy whose repository root cannot be derived carries no repo_root.
#OwnedContentCandy: {
	name!:      string & !="" @go(Name)
	repo_root?: string & =~"^/" @go(RepoRoot)
}

// #OwnedContentReply — what the kind's nodes contribute to that box: files and OCI labels. Both may
// be empty; a kind whose nodes the box does not compose contributes nothing.
#OwnedContentReply: {
	files?: [...#ContentFile] @go(Files)
	// labels — OCI label key -> its JSON value, written into the image's baked labels by the JSON
	// label writer (a structured value, never a Go-quoted string).
	labels?: {[string]: _} @go(Labels,type=map[string]RawBody)
	// diagnostics — what is wrong with the kind's nodes in this scope (an owner that is an image, a
	// body that does not exist where it is derived). Non-empty fails a build and `charly box validate`.
	diagnostics?: [...string] @go(Diagnostics)
}

// #ContentFile — ONE file a kind contributes: where it is on the host, and where it lands relative to
// the image user's home. A file is copied verbatim, so the image holds the authored bytes.
#ContentFile: {
	// source is the absolute host path of the file.
	source!: string & =~"^/" @go(Source)
	// path is the target relative to the image user's home: never absolute, never climbing out.
	path!: string & =~"^[^/]" & !~"(^|/)\\.\\.(/|$)" @go(Path)
	// mode is the file's permission bits (0644 when absent).
	mode?: int & >=0 & <=0o7777 @go(Mode,type=int)
}
