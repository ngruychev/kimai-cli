# kimai-cli

Track time in [Kimai](https://www.kimai.org/) from the command line. A single
static Go binary: no PHP, no Docker, no runtime.

## Install

```sh
go install github.com/anned20/kimai-cli@latest
```

Or build from a checkout:

```sh
go build -o kimai-cli .
```

## Configure

```sh
kimai-cli config init
```

This writes `~/.config/kimai-cli/config.toml` and verifies the credentials
against the instance before finishing.

```toml
url = "https://kimai.example.com"
token_command = "gopass show -o kimai-token"
default_activity = 1
status_format = "{{if .Running}}{{.Project}} {{.CurrentDuration}}{{end}}"
interactive = true
```

Set `interactive = true` to make commands prompt by default, as if `-i` were
given every time. `--interactive=false` overrides it for a single run.

`token_command` runs a shell command to obtain the API token, keeping the
secret out of the config file. Set `token` instead to give it literally, or
export `KIMAI_TOKEN` to override both. `KIMAI_URL` and `KIMAI_CONFIG` override
the instance URL and config path.

## Tracking

```sh
kimai-cli in --project Website --activity Development --description "Fix login"
kimai-cli in --interactive          # prompts for project, activity, description, tags
kimai-cli status
kimai-cli out
```

`clone` restarts a previous entry, carrying over its project, activity,
description, tags and billable flag:

```sh
kimai-cli clone 1234
kimai-cli clone --interactive       # pick a recent entry, then adjust it before it runs
```

`--project` and `--activity` accept an ID, an exact name, or a unique
case-insensitive substring.

## Listing and reporting

```sh
kimai-cli log                          # today
kimai-cli log yesterday
kimai-cli log 2026-08-01 2026-08-24    # inclusive range
kimai-cli report --this-week
kimai-cli report --last-month --project Website
```

Both accept `--json`, `--quiet` (IDs only) and `--format`.

## Editing

```sh
kimai-cli edit 1234 --description "Fix login redirect"
kimai-cli edit current --tags office,home
kimai-cli edit 1234 --begin 09:15 --end 11:30
kimai-cli delete 1234 --force
kimai-cli manual --begin "2026-08-23 09:00" --duration 90m -p Website -a Development
```

`edit` changes only the fields given as flags. `edit --interactive` asks which
of description, project, activity, tags, begin and end to change, then prompts
for each. `current` refers to the running entry, and works with `show`, `edit`
and `delete`.

`delete` prints what it is about to remove, so an entry is never discarded on
the strength of a bare ID.

Tags must already exist in Kimai. An instance that disallows creating tags on
the fly ignores unknown ones without reporting an error, so kimai-cli warns
when a tag it sent was not stored. Run `kimai-cli tag` to see the vocabulary.

Billable state is not settable through the API: Kimai calculates it from the
project, activity and customer configuration.

## Status bars

`status --format` takes a Go template:

```sh
kimai-cli status --format '{{.Project}}: {{.CurrentDuration}}'
kimai-cli status --format '{{.CurrentDuration}} today {{.DailyDuration}}'
```

Run `kimai-cli status --help` for the available fields; `log --help`,
`report --help` and `show --help` list the fields their own entries expose.
Both lists are generated from the rendered types, so they cannot fall out of
date.

`.CurrentDuration` is free. `.DailyDuration` and `.WeeklyDuration` each cost one
extra API call, and `.Project`, `.Customer` and `.Activity` may cost the entity
index. All are fetched only when the template names them, so a template that
omits them stays a single request.

Fetching the token is often the slowest part of a status bar refresh: a
`token_command` that decrypts with GPG can take longer than the API call. Set
`token` in the config file, which is written with 0600 permissions, if the
refresh rate matters more than keeping the secret in a password store. Template functions `truncate`, `join`,
`upper`, `lower` and `default` are available:

```sh
kimai-cli status --format '{{truncate 30 .Description | default "idle"}}'
```

For tmux, in `.tmux.conf`:

```tmux
set -g status-interval 15
set -g status-right '#(kimai-cli status --format "{{if .Running}}{{.Project}} {{.CurrentDuration}}{{end}}") | %H:%M'
```

Guard the template with `{{if .Running}}` so that nothing is printed while the
clock is stopped; without it an idle status bar shows a bare `0s`. Use
`{{else}}` for an explicit idle label.

Set `status_format` in the config to make that the default output of a bare
`kimai-cli status`.

## Scripting

Every listing command supports `--json`:

```sh
kimai-cli log --json | jq -r '.[] | "\(.id) \(.description)"'
kimai-cli status --json | jq -r '.daily_duration'
kimai-cli log --quiet | head -1
```

`status --json` always resolves the daily and weekly totals, so it is a
complete snapshot rather than a lazy one.

The `contrib/` directory holds fzf pickers: `kimai-fzf` selects a recent
description, and `kimai-clone-fzf` clones the matching entry or starts a new
one.

## Timezones

kimai-cli works in this machine's timezone in both directions. `--begin 09:00`
means 09:00 by the local clock, and that entry reads back as 09:00.

Kimai itself stores wall-clock time in the timezone configured on the account,
which need not match this machine's, so writes are converted into it and
responses are converted back. The Kimai web interface shows the account
timezone, so it can display a different clock time for the same entry.
`kimai-cli me` shows which timezone the account uses.

## Metadata

```sh
kimai-cli project
kimai-cli customer
kimai-cli activity --project Website
kimai-cli tag
kimai-cli me
```

## Shell completion

```sh
kimai-cli completion zsh > "${fpath[1]}/_kimai-cli"
```
