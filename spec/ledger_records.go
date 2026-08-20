package spec

// ledger_records.go — the install-ledger record shapes relocated from
// sdk/kit/install_ledger.go + sdk/kit/migrate_support.go (#55 value-type
// relocation spike, cluster 4) and CUE-sourced at schema/deploy.cue (generated
// into cue_types_gen.go — DeployRecord/CandyRecord/StepRecord are wire shapes,
// JSON-persisted to the ledger). kit.* becomes a type/const alias onto these.
// This file holds only the ledger-format version constant.
//
// LedgerSchemaVersion is the install-ledger record format version (the
// ledger-candy-keys cutover's CalVer). It is INDEPENDENT of the project schema
// CalVer (spec/calver.LatestSchemaCalVer) — a non-ledger schema cutover that bumps the
// project HEAD must NOT invalidate the ledger gate.
const LedgerSchemaVersion = "2026.161.1649"
