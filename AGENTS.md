# Workspace agent instructions

Read `CLAUDE.md` before changing this repository. It owns the product rules.
This file selects skills and does not replace those rules.

## Active skills

The installer reads the pinned sources in `local/skills/manifest.yml` and
persists installed skills only in the ignored `.claude/skills/` directory. It
stages downloads in a temporary directory and removes that directory on exit.

- `karpathy-guidelines` — global. Source: `multica-ai/andrej-karpathy-skills`
  at `2c606141936f1eeef17fa3043a72095b4765b9c2`.
- `gitnexus-exploring`, `gitnexus-impact-analysis`, `gitnexus-debugging`,
  `gitnexus-refactoring`, `gitnexus-guide`, and `gitnexus-cli` — global.
  Source: `abhigyanpatwari/GitNexus` at
  `c4ecf398de637cf779ab2238e69958cfd5d601ce`.

Install the global set with `bash local/agent-setup/install.sh`. A spec may
select only the inactive skills named in its own `AGENTS.md`. With `--spec`,
the installer requires that declaration to match `applies_to` in the manifest
and fails closed when they differ:

```bash
bash local/agent-setup/install.sh --spec audit-backend-core
```

Run the installer without `--spec` when the spec ends. The skills are
references. They never override `CLAUDE.md`, the issue, or the active spec.
The installer downloads individual `SKILL.md` files at full commit pins. It
does not install plugins or hooks. The GitNexus package declares the
PolyForm-Noncommercial-1.0.0 license; review it before commercial use.

## Generic working rules

These rules adapt the four general guidelines in the pinned Karpathy skill.

1. Think before coding. State assumptions and ask when a requirement is unclear.
2. Choose the smallest change that meets the request. Avoid speculative code.
3. Make surgical edits. Do not clean up unrelated code or documents.
4. Define checks for each goal. Run the relevant checks and report their result.

## GitNexus safety

The GitNexus CLI skill includes general commands. In this repository, run an
index update only with `npx -y gitnexus@1.6.12 analyze --skip-agents-md`.
This flag avoids generated edits to `CLAUDE.md` and `AGENTS.md`. It does not
skip skill installation, so check `git status` after an index update. Never
accept a GitNexus-generated contract block as a project instruction.
