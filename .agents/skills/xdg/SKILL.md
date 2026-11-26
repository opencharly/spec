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

A charly pod runs as its image user — under `--userns=keep-id` when it has a bind mount — and no
charly pod runs as container root (D-HW-10), so `HOME` is the image's own home and every XDG base directory resolves from it exactly as
it does on the host: `storeroot` sees the same `~/.config/charly`, `~/.local/share/charly`,
`~/.local/state/charly` and `~/.cache/charly` the image user owns, and a volume mounted under that
home is the same tree. Nothing has to be set. Measured inside the image at keep-id as the image
user:

```
$ podman run --rm --userns=keep-id:uid=1000,gid=1000 --user 1000 --entrypoint /bin/sh <image> \
    -c 'echo HOME=$HOME; id -un; id -u; echo XDG=[$XDG_CONFIG_HOME][$XDG_DATA_HOME][$XDG_CACHE_HOME][$XDG_STATE_HOME]; python3 -c "import os;print(os.path.expanduser(chr(126)))"; node -p "require(chr(111)+chr(115)).homedir()"'
HOME=/home/user
user
1000
XDG=[][][][]
/home/user
/home/user
```

Do not set `XDG_*` in a deploy: they are derived from `HOME`, and an explicit value overrides an
operator's choice and any image that sets its own. And do not set `HOME` to compensate for a user
that resolves the wrong one — a workload whose home is not the image home is running as the wrong
user, which is the defect to fix (D-HW-10), not a variable to paper over. A process started by
supervisord or a service wrapper inherits the image's `HOME`; a wrapper that falls back to
`${HOME:-/home/user}` is agreeing with the image, not overriding it.

## Application model caches

A model cache is a cache root, so it belongs at `$XDG_CACHE_HOME/<app>` and must survive a
container re-create. The two halves are separate: the PATH convention is the application's, and
the PERSISTENCE is a charly `volume:` in the candy.

Measured in the `openwebui` image (`huggingface_hub` 1.33.0, `sentence-transformers` 5.5.1),
whose hub cache is the concrete instance:

```
$ XDG_CACHE_HOME=/x/y python3 -c 'import huggingface_hub.constants as c; print(c.HF_HOME)'
/x/y/huggingface
$ HF_HOME=/x/z python3 -c 'import huggingface_hub.constants as c; print(c.HF_HOME)'
/x/z
```

So `huggingface_hub` resolves `HF_HOME` first and otherwise `$XDG_CACHE_HOME/huggingface`, with
`XDG_CACHE_HOME` defaulting to `~/.cache` (`constants.py:160-167`). The widely repeated claim that
HuggingFace ignores `XDG_CACHE_HOME` is wrong for this version — the variable does move the cache.
Inside a pod `XDG_*` are unset and `HOME` is the image home (measured `HOME=/home/user`), so the
cache lands at `~/.cache/huggingface`, i.e. in the container's writable layer.

To keep it warm, declare a volume over the cache home the app already uses rather than pinning
`XDG_CACHE_HOME`/`HF_HOME` in the deploy:

```yaml
volume:
  - name: cache
    path: "${HOME}/.cache"     # the resolver expands ${HOME} from the image home
```

`kit.ExpandPath` (`spec/spec/env_path.go`) expands `~`, `${HOME}` and `$HOME` for a candy volume
path, so the mount and the app's own `expanduser("~")` agree without a variable. `pod-openwebui`
declares it beside its `data` volume, and `opencharly/pod-openwebui#36` is the measured failure it
fixes: with only `/opt/data` persisted, a re-created container re-fetched the 30-file embedding
model (889 MB, 254 s to the first `200`).

An application that does NOT follow XDG gets its own documented root instead, never an invented
`${HOME}/.cache/<app>` path it will not read: `ollama` writes `~/.ollama` (`OLLAMA_MODELS`), and
`immich` keeps its ML models on a volume at `~/.immich/models` (named through `HF_HOME`, or
`TRANSFORMERS_CACHE` in releases that predate the rename — the volume is what persists either
way). A `torch`-based trainer may read only `HF_HOME`/`TORCH_HOME`, so check the installed source
before choosing. Whichever root is used, the witness is a runtime check that the cache directory
is a MOUNT POINT (`grep " $cache " /proc/self/mountinfo`), and that the app's own resolver names a
path under it; requiring the directory to be non-empty is only valid once the app has run, because
a bed that never re-creates a container cannot otherwise detect the regression.
