# `cmd add --group` Design

Date: 2026-08-02

## Context

`min cmd add <name>` always creates the final path segment as a **leaf** command
(a struct with a `Run` method). A leaf cannot also be a command group, so:

```
$ min cmd add z
$ min cmd add z.s
min: error: cannot add subcommands under "z": ZCmd is already a leaf command
```

There is no way to first create a bare group and then add leaves under it.
Users who run `min cmd add admin` expecting a namespace are stuck.

## Goal

Add an explicit `--group` flag to `min cmd add` so users can create a command
group (an empty struct with no `Run` method) and later add leaf commands under
it.

## Design

### Interface

`min cmd add --group <name>` creates the named command as a group.

- `min cmd add --group z` → `type ZCmd struct {}` (empty, no `Run`), registered
  as `Z ZCmd \`cmd:"" help:"z command group"\`` in the CLI/parent struct.
- Dot-notation makes every segment a group: `min cmd add --group a.b` creates
  group `a` and group `b`.
- A `--desc` value is used as the final group's help text; the default is
  `"<name> command group"`.
- Error when the name already exists as a leaf: `cannot add group "z": ZCmd is
  already a leaf command`.
- Adding an existing group again is a silent no-op (consistent with how
  `cmd add` already treats existing intermediate groups).
- Without `--group`, behavior is unchanged.

### Implementation

- `CmdAddCmd` gains a field:
  ```go
  Group bool `help:"Create a command group instead of a leaf command"`
  ```
  Kong exposes it as `--group`.
- `scaffold.AddCommand(name, desc string, group bool) error`:
  - `isFinal := i == len(segments)-1`
  - `isLeaf := isFinal && !group`
  - Final-group help: `segHelp = helpText` where `helpText` defaults to
    `leaf + " command group"` when `group` is set, else `leaf + " command"`.
  - The "already a leaf" error message differs when `group` is set
    (`cannot add group %q: %s is already a leaf command`).
- `cmd.CmdAddCmd.Run` passes `c.Group` through.

### README

Add a `--group` example to the "Add a command" section.

## Testing

- `--group z` produces an empty `ZCmd` group; a subsequent `cmd add z.s` succeeds.
- `--group z` where `z` is an existing leaf errors with the new message.
- `--group a.b` creates both segments as groups.
- `--desc` is honored for the final group.
- `--group z` twice is a no-op success.
- All existing `cmd add` tests pass unchanged.

## Out of Scope

- No `cmd rm`/delete command.
- No auto-conversion of existing leaves into groups.
