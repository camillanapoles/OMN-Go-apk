# 0030. Adopt the agent skill program

* Status: accepted
* Version: 26.11.35
* Code: `CLAUDE.md`, `doc/decisions/0030-adopt-the-agent-skill-program.md`

## Context

The standing instructions of `CLAUDE.md` already name two skills. The
`ste-writing` skill writes Simplified Technical English. The `github-ops-cicd`
skill runs the GitOps loop. No file of the repository holds these skills, and no
file names how a session obtains them. A session that starts from a fresh
checkout reads the two names and cannot know what the skills hold, where they
come from, or how to get them. The skill supply is implicit in the tool
environment. That breaks the rule that each installation must be reproducible.

Four repositories outside this tree hold skills for the layers of this project:

1. `multica-ai/andrej-karpathy-skills` holds generic behavioral guidelines for
   an agent session.
2. `mattpocock/skills` holds skills for software engineering work.
3. `chrisbanes/skills` holds skills for Kotlin, Jetpack Compose and Android
   development.
4. `affaan-m/ECC` holds the harness layer: skills, instincts and memory for the
   agent runtime.

A fifth source serves the harness only. `jev-ai/jev-agent-skill` holds one
skill for typed decisions. It sends a state and typed questions to the Jev AI
service outside the tree, and it gets a probability-backed answer that the
session can branch on. It calls the service with an API key of the person who
runs the session. The key stays in the local environment and never enters the
repository. This skill stays inactive by default, and a stage selects it only
when a judgment of the stage needs it.

Each of the five is active and maintained, and each targets one layer. The
first four cover the layers of this project without overlap. The maintainer
asked for one standing process that adds skills in a controlled way, gives
each workspace one `AGENTS.md`, and keeps the whole program reproducible.

## Decision

Adopt one agent skill program for this repository and for each workspace that
opens it. The program holds three stages.

### Stage 1. Verify the source, and install only what passes

1. Verify that a plugin or skill source is valid before you use it. Check that
   the repository is reachable, that it holds a manifest or a skill file, and
   that its last change is recent.
2. Check each skill and each framework for the whole workspace, not only for
   one task.
3. Install each skill through a reproducible path. Accept an `install.sh`
   script that the source provides, or a committed tree of the skill files.
   Record the commit of the tree that you install. Do not install from an
   unversioned download.
4. Check the hooks of a plugin before you enable it. A hook that changes the
   prompt, the tool list or the session flow is named in the spec.
5. Post the spec of each skill in the skills patterns of the project. Activate
   the global skills of the project in one place.
6. Install the general skills at once, because they apply to every task. A
   skill that applies to one stage only stays inactive until that stage starts.

### Stage 2. Stage the skill increments

1. A new skill does not go into the initial instructions of a session. The
   initial instructions keep the rules that every session needs.
2. A script or a note that the current process adds carries each new skill.
   The script names the skill, its source and its commit.
3. Split each increment into two parts. The first part is the global set: the
   skills that every workspace will have. The second part is the stage set: the
   skills that one stage of work needs.
4. A global skill becomes active only after the maintainer accepts it into the
   global set.

### Stage 3. Select the stage set for each spec

1. Every spec carries the general skills that are already active, and no more.
2. When a spec starts, the workspace instruction selects from the inactive
   skills the ones that the focused development of that spec needs.
3. The selection names each skill, its source and its commit, in the spec or in
   the `AGENTS.md` of the workspace.
4. When the spec ends, the stage skills return to inactive. The global set does
   not grow because one spec used a skill.

### Interoperability principles

The program keeps each layer independent.

1. A skill from one source must not change the behavior of another source.
2. A skill must use the file formats and the hooks of the tool that runs it.
3. The skills of the five sources keep their own names and their own files.
   The project copies or adapts a skill only when a copy is necessary, and it
   names the source of the copy.
4. `AGENTS.md` holds the rules of the project and of the workspace. It carries
   the generic appendix of the karpathy guidelines, and it stays short.

Each project, each workspace and each spec holds one `AGENTS.md`. The file
holds the instructions that the session must read, and the skills that are
active in that workspace. A generic appendix of the karpathy guidelines gives
the behavioral base: think before coding, simplicity first, surgical changes,
and goal-driven execution. The `AGENTS.md` of a spec adds the stage skills of
that spec.

## Rejected alternatives

* **Vendor every skill into the repository tree.** A large skill tree adds
  files that no build uses. The Docker build and the APK grow, and each skill
  update becomes a change to this repository.
* **Let each session choose its skills with no record.** Two sessions then run
  with two different skill sets, and a task that depends on a skill can fail
  for no named reason.
* **Put every skill in the initial instructions.** The initial instructions
  grow with each skill, and a session that needs one skill reads all of them.
* **Install skills from a moving branch.** The installed tree changes without a
  record. A later session cannot reproduce the tree of the first session.

## Consequences

* Each installation is reproducible. The spec names the source and the commit
  of the installed tree.
* The global set stays small, and the maintainer accepts each global skill
  once.
* Each spec carries only the skills that it needs. The general skills stay
  active, and the stage skills return to inactive after the spec.
* Each workspace holds one `AGENTS.md`, with the active skills and the generic
  appendix of the karpathy guidelines.
* A new skill enters the session through a script or a note of the current
  process, never through the initial instructions.
* The five skill sources stay independent. A change in one source does not
  change the behavior of the others.
