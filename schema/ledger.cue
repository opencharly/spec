// ledger.cue — the `ledger:` kind: the per-host INSTALL LEDGER.
//
// What is installed on this host (deploy records + per-candy artifact records)
// lives in the per-host charly.yml (~/.config/charly/charly.yml) under `ledger:`,
// replacing the per-deploy JSON files under ~/.config/opencharly/installed/
// (deploys/<deploy-id>.json + layers/<candy>.json — both deleted by the cutover).
// One file, one schema, one validation path.

// #LedgerConfig is the top-level `ledger:` block. `deploys` maps deploy-id →
// #DeployRecord; `candies` maps candy name → #CandyRecord.
#LedgerConfig: {
	deploys?: {[string]: #DeployRecord} @go(Deploys)
	candies?: {[string]: #CandyRecord} @go(Candies)
}
