# lazytuck

A lazygit-style terminal UI for [Tuckr](https://github.com/RaphGL/Tuckr) dotfile repositories.
See the link state of every single file, diff what drifted, adopt or restore it with one key,
and commit and push with a secret scan in front. Works on macOS and Linux.

```
╭─ 1 Groups ───────────────────────────────╮╭─ 3 Detail ─────────────────────────────────────────────────────╮
│ ! claude                             0/8 ││Path      .config/tmux/tmux.conf                                │
│ ! codex_macos                        0/1 ││Group     tmux                                                  │
│ ! ideavim                            0/1 ││State     ± drift                                               │
│ ! npm                                0/1 ││Target    /home/example/.config/tmux/tmux.conf                  │
│ ! nvim                              0/14 ││Source    /home/example/.dotfiles/Configs/tmux/.config/tmux/tmux│
│ ! nvim-vscode                        0/1 ││                                                                │
│>! tmux                               0/1 ││Keys      a adopt (home → repo) · r restore (repo → home)       │
╰──────────────────────────────────────────╯│                                                                │
╭─ 2 Files · tmux ─────────────────────────╮│Diff  repo ↔ home                                               │
│ ± .config/tmux/tmux.conf                 ││--- repo:tmp/lzt/home/.dotfiles/Configs/tmux/.config/tmux/tmux.c│
│                                          ││+++ home:tmp/lzt/home/.config/tmux/tmux.conf                    │
│                                          ││@@ -46,3 +46,4 @@ set -g status-right "[MODE: #{?client_prefix,#│
│                                          ││ # Initialize TMUX plugin manager (keep this line at the very bo│
│                                          ││ run '~/.tmux/plugins/tpm/tpm'                                  │
│                                          ││                                                                │
│                                          ││+set -g mouse on  # local edit                                  │
╰──────────────────────────────────────────╯│                                                                │
╭─ 4 Git ──────────────────────────────────╮│                                                                │
│ main → origin/main ↑0 ↓0                 ││                                                                │
│ clean                                    ││                                                                │
│                                          ││                                                                │
│                                          ││                                                                │
│                                          ││                                                                │
╰──────────────────────────────────────────╯╰────────────────────────────────────────────────────────────────╯
 a adopt (home → repo) · r restore (repo → home) · ? help
```

## Why

Tuckr keeps dotfiles as plain files in a repo and symlinks them into `$HOME`, with
per-OS groups (`zsh_macos`, `zsh_linux`). Its `status` only reports whole groups, though.
lazytuck shows each file and what to do about it:

| State | Meaning |
|---|---|
| `✓ linked` | the target is a symlink to the repo file |
| `○ missing` | nothing at the target yet |
| `= same` | a real file with identical content (e.g. left behind by chezmoi) |
| `± drift` | a real file that differs from the repo — the diff is shown |
| `✗ foreign` | a symlink to somewhere else, a directory, or another file type |
| `↯ dangling` | a symlink to nothing |
| `↑ shadowed` | a more specific group (`_macos` > `_unix` > base) provides the same path |
| `· inactive` | the group's OS suffix does not match this machine |

The repo stays a plain Tuckr repo. lazytuck writes no metadata into it, so `tuckr` keeps working.

## Install

```sh
brew install --cask t1mdotcom/tap/lazytuck     # macOS and Linux
```

or with Go 1.26+:

```sh
go install github.com/t1mdotcom/lazytuck/cmd/lazytuck@latest
```

## Use

```sh
lazytuck                  # open the TUI
lazytuck status           # per-file table; exit 0 = all linked, 1 = something to fix, 2 = error
lazytuck status --json    # the same as JSON
lazytuck --repo ~/dots    # skip the lookup
```

The repo is found the way Tuckr finds it: `$TUCKR_HOME/dotfiles`, then
`~/Library/Application Support/dotfiles` (macOS) or `~/.config/dotfiles` (Linux), then `~/.dotfiles`.
Files are linked into `$TUCKR_TARGET`, or `$HOME`.

### Keys

| Key | Action |
|---|---|
| `tab` `1`–`4` | switch pane (Groups, Files, Detail, Git) |
| `j` `k` | move, or scroll the detail pane |
| `space` | link / unlink a file; on a group: link all or unlink all |
| `a` | adopt: copy the home version into the repo, then link |
| `r` | restore: back up the home version, link the repo version |
| `d` | jump to the diff of a drifted file |
| `n` | add a file or directory from `~` to a group (new groups may use `_macos` etc.) |
| `c` | commit all changes |
| `p` / `P` | push (asks first) / pull with `--ff-only` |
| `R` | rescan |
| `?` | help |

## Safety

- **Backups first.** Whatever lazytuck replaces in `$HOME` is copied to
  `~/.local/state/lazytuck/backup/<timestamp>/` (or `$XDG_STATE_HOME`) before anything changes.
  If the backup fails, the operation does not run.
- **No gap.** Links are created under a temporary name and renamed over the target, so a
  file is never missing — programs that watch their config (AeroSpace with
  `auto-reload-config`, for example) never see it disappear.
- **Hands off the repo.** If a path in `$HOME` resolves into the repo through a symlinked
  directory, lazytuck refuses to touch it. Unlink only removes symlinks that point into `Configs/`.
- **Secret scan.** Before a commit, lazytuck scans exactly what `git add -A` would record; before
  a push, it scans the patch of every outgoing commit, so a token that was added and removed
  again is still caught. Hits (AWS, GitHub, GitLab, SonarQube, Slack, OpenAI-style keys,
  private keys, `password=`/`token:` assignments) block until you type `yes`.

## Using Tuckr alongside

Run Tuckr with `--only-files`:

```sh
tuckr add -y --only-files '*'
```

Without it, Tuckr 0.13.1 links whole directories, and when a base group and its `_macos`/`_linux`
group share a directory the result depends on the run: in 20 identical runs, 4 produced missing
links, links into the wrong group, or symlinks written into the repo. With `--only-files`,
100 of 100 runs were correct. lazytuck itself always links single files.

Hooks (`Hooks/<group>/post.sh`) are listed per group but not run; use `tuckr set` for that.
Not supported yet: Tuckr profiles, `%ENV` path segments (shown as unsupported and left alone),
encrypted `Secrets/`.

## Development

```sh
make check      # gofmt, go vet, go test -race
make build      # bin/lazytuck
```

Requirements and invariants live in [`SPEC.md`](SPEC.md); every task is committed against it.
Releases: `scripts/release.sh <version>` tags `main`, publishes the GitHub release with GoReleaser
and updates the cask in [`t1mdotcom/homebrew-tap`](https://github.com/t1mdotcom/homebrew-tap).

## License

MIT
