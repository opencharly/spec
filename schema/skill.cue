// CUE schema for the `skill` KIND — a first-class harness skill entity (the migration of the
// plugins/ skill corpus into candy configs). A `skill:` node is a sibling top-level entity in a
// candy's charly.yml (ParseDoc iterates every top-level node, each with its own single kind
// discriminator), so a candy file stacks its `candy:` node plus the `skill:` nodes it owns.
//
// The skill's BODY lives in a FILE (the `.agents/skills` relocation). A skill is authored at
// <tree>/<name>/SKILL.md plus references/<stem>.md, where <tree> is the owning candy's `skills:`
// list (default `.agents/skills`) and <name> is this node's `name:` — or the NODE NAME when
// `name:` is omitted. An `agent`-type node authors <tree>/<name>.md instead.
//
// This node therefore carries the ROUTING the consumers need — family (which corpus family the
// skill lands in), owner (the OCI-label association), triggers (the R0 dispatcher rows), category —
// while `description:`/`content:`/`references:` are the TRANSITIONAL in-line form: an entity still
// carrying them is read from here; once its body has moved to the file they are OMITTED and the
// file is the source of record. A repo mid-move may carry both, and the read order while both
// exist is the generator's contract (`/charly-internals:marketplace`), not this schema's. The
// in-line form is deleted outright by the relocation's final leg.
//
// The marketplace generator (candy/plugin-marketplace, command:marketplace) synthesizes
// <family>/skills/<name>/SKILL.md + references/*.md; CollectSkills (sdk/deploykit) projects the
// composed candies' skills into the `ai.opencharly.skill` OCI label so a built image is
// self-describing (readable via `charly box labels` / `charly deploy from-box`, no external fetch).
//
// owner is the candy/concept-candy entity name that owns this skill — the build-time association
// that decides which images carry it (CollectSkills filters by owner ∈ composed candy chain).
// family is the marketplace family (the corpus directory name) the generator groups by.
//
// type: "agent" marks a sub-agent definition (the former <family>/agents/*.md) — one
// unified doc kind, same lifecycle; model/tools carry the agent frontmatter fields.
#Skill: close({
	name?:       string & =~"^[a-z][a-z0-9-]*$" // skill id = the SKILL.md directory name; DEFAULTS to the node name
	family:      string & =~"^[a-z][a-z0-9-]*$" // marketplace family → <family>/skills/
	owner:       string & =~"^[a-z][a-z0-9-]*$" // owning candy/concept-candy entity name
	description?: string & !=""                  // SKILL.md frontmatter description — authored in the FILE once the body moved
	content?:    string & !=""                   // SKILL.md markdown body — authored in the FILE once the body moved
	type?:       *"skill" | "agent"              // "agent" ⇒ a sub-agent definition (frontmatter adds model/tools)
	model?:      string & !=""                   // agent type: frontmatter model (default "inherit")
	tools?:      [...(string & !="")]            // agent type: allowed tool set
	references?: [...#SkillReference]            // SKILL.md references/<stem>.md split files — authored in the FILE once the body moved
	triggers?:   [...(string & !="")]            // R0-dispatcher trigger phrases (generated dispatcher table rows)
	category?:   *"development" | "commands" | "kind" | "images"
})
#SkillReference: close({
	name:    string & =~"^[a-z0-9][a-z0-9-]*$" // file stem (references/<name>.md)
	content: string & !=""
})
