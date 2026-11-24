---
name: xdg
description: Resolve host storage paths through spec/storeroot. Use when choosing a root, handling configuration or data search paths, or testing XDG and Charly overrides.
---

# Resolve a host path

Choose the root by the lifetime of the content. Configuration is operator input. Data holds durable keys, VM disks and bind-mounted volumes. State holds operating history. Cache holds objects that can be rebuilt and evicted. Runtime holds per-session files such as sockets.

Call `github.com/opencharly/spec/storeroot.Dir` with the matching root and propagate its error before joining a child name. Consumers already using the SDK can use its storekit re-export. Leave home-directory discovery, environment parsing and the application directory suffix to the resolver. DESIGN requirements D-STORE-1, D-STORE-11 and D-STORE-12 govern placement.

For configuration or data lookup, use `Search(root, rel)` with a relative name containing no parent component. Handle the returned found flag. Use `Dirs(root)` when the operation needs all candidate directories in precedence order. Both start at the user's Charly directory. A valid `CHARLY_CONFIG_DIRS` or `CHARLY_DATA_DIRS` supplies complete application directories for the remaining entries. Otherwise the resolver uses the XDG search list.

A valid Charly root override names the complete directory. For example, if `CHARLY_DATA_HOME=/srv/charly-data`, `DataDir()` returns `/srv/charly-data`. If only `XDG_DATA_HOME=/srv/data` is set, it returns `/srv/data/charly`. For cache, `CHARLY_CACHE_HOME` precedes the compatibility override `CHARLY_CACHE_DIR`. The separate `CHARLY_REPO_CACHE` override moves only the remote repository directory.

Exercise changed callers with temporary roots and assert the actual artifact path. For configuration and data discovery, populate competing entries and verify that the user entry wins. The resolver's implementation and precedence tests live together in the spec module's `storeroot/` package.

## Inside a pod

The roots resolve the same way in a container, from `HOME`: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_CACHE_HOME` and `XDG_STATE_HOME` are HOME-derived by specification, and `storeroot` derives from them. A deploy that runs as **container root** — the CDI GPU device plus a host-owned volume shape, where rootless `userns=host` maps container root onto the invoking user and every other uid into the subordinate range — has `HOME=/root` by default while its volumes are mounted under the image user's home. A process that resolves `HOME` then looks in `/root` and misses the mounted data, and every XDG base directory moves with it. The quadlet generator pins `HOME=<ai.opencharly.home>` for exactly that branch, so no candy and no image needs to know the answer.

Do not set `XDG_*` in a deploy to compensate: they are derived from `HOME`, and an explicit value overrides an operator's choice and any image that sets its own. Measured in that shape with `HOME=/home/user`: every `XDG_*` variable is correctly unset, and `python` `expanduser("~")` and `node`'s `os.homedir()` both report `/home/user`. A deploy with only named volumes keeps the image user, where `HOME` is already the image home.
