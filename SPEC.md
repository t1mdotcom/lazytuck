# lazytuck — SPEC

## §G Goal

lazygit-style TUI for Tuckr dotfile repos. See per-file link state, diff drift, adopt/restore/link per keypress, commit+push w/ secret scan. macOS + Linux. Repo stays usable w/ plain `tuckr`.

## §C Constraints

- Lang: Go, single static binary. TUI: Bubble Tea + Lip Gloss (+ Bubbles). Targets: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64.
- Repo layout = Tuckr layout (§I.layout). ⊥ own metadata files in repo. ∃ user w/o lazytuck → `tuckr add -y --only-files '*'` still works.
- `tuckr status --json` group-level only (`symlinked`/`not_symlinked`/`conflicts`) ∴ per-file state computed natively from FS. `tuckr` binary ⊥ required at runtime.
- File-level symlinks only. Reason: Tuckr 0.13.1 dir folding w/ base + `_<os>` group sharing dir → nondeterministic result (measured 2026-10-09: 4/20 runs wrong w/o `--only-files`, 100/100 ok with).
- Out of scope v1: Tuckr profiles (`dotfiles_<profile>`), `%ENV` path segments (shown as unsupported, ⊥ touched), `Secrets/` encryption, running hooks (user runs `tuckr set`), Windows.
- Git via `git` CLI subprocess. ⊥ go-git.
- Deps: Charm libs + stdlib. New dep ! entry here.
- UI text English (public tool). Code, comments, commit subjects English.
- Dev box: macOS, user's real repo `~/.dotfiles` (13 groups, 33 files). Tests ⊥ touch real `$HOME`.
- Distribution: GitHub Releases via GoReleaser (local `scripts/release.sh`, token = `gh auth token`) + Homebrew cask `Casks/lazytuck.rb` in `t1mdotcom/homebrew-tap` (GoReleaser `homebrew_casks`; `brews`/formula deprecated since GoReleaser v2.16, casks run on Linuxbrew). Unsigned ∴ cask post-install hook strips `com.apple.quarantine` on macOS. Install: `brew install --cask t1mdotcom/tap/lazytuck`.

## §I Interfaces

- cmd: `lazytuck` → TUI on resolved repo.
- cmd: `lazytuck status [--json]` → per-file table | JSON `[{group,path,target,state}]`; exit 0 iff ∀ active files `linked` (`inactive`/`shadowed`/`unsupported` ignored), 1 ∃ `missing`|`same`|`drift`|`foreign`|`dangling`, 2 error.
- cmd: `lazytuck --repo <dir>` → override repo lookup.
- repo lookup (= Tuckr `get_dotfiles_path`): `--repo` > `$TUCKR_HOME/dotfiles` (if exists) > `<config_dir>/dotfiles` (macOS `~/Library/Application Support/dotfiles`, Linux `$XDG_CONFIG_HOME/dotfiles` | `~/.config/dotfiles`) > `~/.dotfiles`. None → exit 2 w/ message listing tried paths.
- target dir: `$TUCKR_TARGET` (non-empty) > `$HOME`.
- layout: `Configs/<group>/<rel>` → `<target>/<rel>` exact, ⊥ path rewriting. Suffix `_<t>` w/ `<t>` ∈ Tuckr `VALID_TARGETS`: active iff `_macos` on darwin, `_linux` on linux, `_unix` on both; `_wsl` active iff linux & `/proc/version` contains `microsoft`; other valid suffix → inactive; unknown suffix → plain group name. `Hooks/<group>/{pre,post}*` listed read-only. `Secrets/` ignored.
- precedence (= Tuckr `get_group_priority`): base 0 < `_unix` 1 < `_<os>` 2 < `_wsl` 3. Same `<rel>` in >1 active group → highest priority wins; tie → group name asc, last wins (deterministic). Losers → `shadowed`.
- state (per file): `linked` (symlink → repo file, | real path ∈ repo via dir symlink = folded) | `missing` (target absent) | `same` (real file, bytes = repo) | `drift` (real file, bytes ≠ repo) | `foreign` (symlink → elsewhere | dir | other non-file) | `dangling` (symlink → nonexistent) | `inactive` (OS suffix ≠ runtime) | `shadowed` (lost precedence) | `unsupported` (`%ENV` segment).
- ops (per file | per group = ∀ files):
  - link: `missing` → create symlink. `same` → backup real file, link.
  - unlink: `linked` → remove symlink, ⊥ other states.
  - adopt: `drift` → copy home bytes → repo, backup, link.
  - restore: `drift`|`foreign`|`dangling` → backup, link (repo wins).
  - add: path in `~` not in repo → pick group (existing | new, optional `_<os>`) → move into `Configs/<group>/<rel>`, link.
  - unmanage (`x`): ∀ state except `InRepo` → remove file from repo. `linked` → home symlink replaced by real copy first (atomic). Other states: home ⊥ touched.
  - delete (`X`): ∀ state except `InRepo` → remove file from repo and our copy from home: `linked` → remove symlink; `same` → backup, remove. `drift`/`foreign`/`dangling`/`missing`/`inactive`/`shadowed`/`unsupported` → home ⊥ touched.
  - repo removal: backup repo file first, then remove empty parent dirs up to `Configs/` (exclusive). Group dir gone → `Hooks/<group>` moved to backup.
  - group ops on `(all)` pseudo-group: ⊥ unmanage, ⊥ delete.
- backup: `~/.local/state/lazytuck/backup/<YYYYMMDD-HHMMSS>/<rel>` (respects `$XDG_STATE_HOME`); repo files under `…/<YYYYMMDD-HHMMSS>/repo/Configs/<group>/<rel>`, hooks under `…/repo/Hooks/<group>`.
- panes: 1 Groups (active/inactive, counts per state) · 2 Files (state glyph + path) · 3 Detail (target, state, diff `repo ↔ home` for `drift`) · 4 Git (branch, ahead/behind, changed files).
- keys: `tab`/`1-4` focus · `j/k` move · `space` link|unlink · `a` adopt · `r` restore · `x` unmanage · `X` delete · `d` diff · `n` add file · `c` commit · `p` push · `P` pull · `R` rescan · `?` help · `q` quit. Confirm prompt: `r`/`x`/`X` on file, ∀ group op (≥1 file), `p`. Commit: message `enter` = confirm.
- secret scan: added lines only. Commit → diff `git add -A` would record, built in temp `GIT_INDEX_FILE` (real index ⊥ touched). Push → patch of ∀ outgoing commit (`git log -p @{u}..HEAD`), ∴ secret added then removed still caught. Patterns: `AKIA[0-9A-Z]{16}`, `gh[po]_[A-Za-z0-9]{16,}`, `glpat-[A-Za-z0-9_-]{16,}`, `squ_[0-9a-f]{20,}`, `sk-[A-Za-z0-9_-]{20,}`, `xox[abp]-[A-Za-z0-9-]{10,}`, `-----BEGIN [A-Z ]*PRIVATE KEY-----`, `(?i)(_password|_authtoken|password|passwd|secret|token)["']?\s*[=:]\s*(\S+)` (value ⊥ empty, ⊥ `$VAR`/`<…>`). Min length after prefix: avoids `task-`-style false hits. Hit → list `file:line` + masked excerpt, block; override ! typed `yes`.

## §V Invariants

- V1: ∀ op replacing/removing real file in `~` → backup first (§I.backup). Backup fail → op aborted, file untouched.
- V2: unlink removes symlink only if it resolves into repo `Configs/`. ⊥ delete real files.
- V3: ∀ link = file symlink, absolute target `<repo>/Configs/<group>/<rel>`. ⊥ dir symlinks. Parent dirs in `~` created as real dirs.
- V4: ⊥ symlinks created inside repo. Repo writes only via adopt | add.
- V5: scan & status read-only. TUI start, rescan, `lazytuck status` ⊥ mutate FS.
- V6: path mapping `Configs/<group>/<rel>` → `~/<rel>` byte-exact ∀ rel incl. non-dot (`Library/…`, `bin/…`) & spaces. Test-covered.
- V7: inactive group (OS mismatch) ⊥ linked by any op, incl. per-group ops.
- V8: op error → status bar shows reason, state rescanned. ⊥ silent fail. Per-file op atomic: link via temp symlink + `rename`.
- V9: git commit/push only on explicit key + confirm. ⊥ auto-commit, ⊥ auto-push.
- V10: secret scan hit → commit/push blocked until override (§I.secret scan). Scan runs before `git commit`, not after.
- V11: same input FS → same status output (deterministic order: group asc, path asc).
- V12: tests use temp `$HOME` + temp repo. ⊥ real `$HOME` in tests.
- V13: target location (parent dirs resolved, final component not followed) ∈ repo (folded dir symlink) → `InRepo`; = source → state `linked` (folded), ≠ source → `foreign`. ∀ op ⊥ move/replace/remove `InRepo` target. Symlink in `~` pointing into repo ⊥ `InRepo` (replacing it is safe). Reason: backup+replace would move repo file out of repo.
- V14: release only from clean `main` = `origin/main`, after `make check` passes. Tag `v<semver>` = archive version = cask version.
- V15: ∀ repo file removal → backup copy first; backup fail → op aborted, repo + home untouched. Removal ⊥ commits (V9).
- V16: unmanage of `linked` → home real copy in place before repo file removed (⊥ dangling window). Delete touches home only for `linked` (non-folded) and `same`.

## §T Tasks

id|status|task|cites
T1|x|Go module `github.com/t1mdotcom/lazytuck`, `cmd/lazytuck`, `internal/{repo,state,ops,gitx,secrets,tui}`, Makefile, CI matrix ubuntu+macos (`go vet`, `go test -race`)|§C
T2|x|`repo`: lookup order, target dir, enumerate groups/files, suffix → active/inactive, `%ENV` → unsupported, tests|I.repo lookup,I.target dir,I.layout,V6,V7,V12
T3|x|`state`: per-file classify incl. precedence → `shadowed`, deterministic sort, tests ∀ state|I.state,I.precedence,V5,V6,V11,V12
T4|x|`lazytuck status [--json]` + exit codes, smoke vs `~/.dotfiles`|I.cmd,V5,V11
T5|x|`ops`: link/unlink/adopt/restore w/ backup + atomic rename, tests ∀ op × state|I.ops,I.backup,V1,V2,V3,V4,V7,V8,V12
T6|x|TUI shell: 4 panes, focus, nav, help overlay, rescan|I.panes,I.keys,V5
T7|x|TUI ops wiring: space/a/r + confirm prompts + status bar errors|I.ops,I.keys,V1,V8
T8|x|diff view `repo ↔ home` for `drift` in Detail pane|I.panes
T9|x|add-file flow: path picker under `~`, group picker/new group w/ `_<os>`, move + link|I.ops,V1,V4
T10|x|`gitx`: status, ahead/behind, commit w/ message, pull, push; Git pane|I.panes,V9
T11|x|`secrets`: scan staged/outgoing diff, block + override; tests w/ fixtures ∀ pattern|I.secret scan,V10
T12|x|`.goreleaser.yaml` (4 targets, tar.gz, checksums, `homebrew_casks` → `Casks/lazytuck.rb`), `scripts/release.sh`, snapshot build verified|§C,V14
T13|x|README (install, keys, Tuckr compat, `--only-files` rationale) + portfolio entry|§G
T14|x|`ops`: unmanage/delete per file, repo backup, empty-dir + hook cleanup, tests ∀ op × state|I.ops,I.backup,V13,V15,V16,V12
T15|~|TUI `x`/`X` on file + group (⊥ `(all)`), confirm, help, README|I.keys,I.ops,V9
T16|.|release v0.2.0, verify `brew upgrade` macOS + Linux|V14

## §B Bugs

id|date|cause|fix
B1|2026-10-09|`InRepo` from symlink destination → ∀ linked file flagged; V13 "real path" ambiguous|V13
