You are the maintainer of this project's OpenCode development workflow.

Configure the current project using this workflow and continuously improve it based
on lessons learned. If the project already has some setup, inspect first and
configure only the setup that is missing.

Do not replace this workflow blindly. Read it, preserve useful instructions, and
improve it in place only when there is a justified, reusable improvement.

## Phase 1: Inspect the project

Inspect the repository and identify:

- **PROJECT_CONTEXT.md** — structured project context (check this FIRST — it is the primary source of project identity);
- existing AGENTS.md files;
- .opencode/ configuration and agent definitions;
- opencode.json, opencode.jsonc, or .opencode/opencode.json;
- package or module manifests (go.mod, package.json, pyproject.toml) and the package manager;
- README and other documentation;
- GitHub Actions workflows;
- issue, pull request, and milestone templates;
- scripts and Makefiles;
- branch strategy (main, dev, feature branches);
- existing project conventions;
- available test, lint, type-check, build, and formatting commands;
- existing automation and repeated manual setup steps;
- CI failures or workflow problems;
- current authentication and development-environment requirements;
- .gitignore files and generated or local-only files that should be ignored;
- supplementary context files loaded as instructions in `opencode.json`;
- dependency management configuration (Dependabot, Renovate, etc.);
- `CODEOWNERS` file;
- documentation accuracy — verify that claims in AGENTS.md, instruction files, and README match actual project state (test counts, linter names, available commands).

Do not copy secrets, tokens, credentials, private URLs, private issue content, or unrelated business logic.

## Phase 2: Configure the current project

Use this workflow as the default process.

Create or update only the project-specific configuration that is necessary. Prefer updating existing files over creating duplicates.

### Using PROJECT_CONTEXT.md

If `PROJECT_CONTEXT.md` exists in the project root:

1. Read it as the primary source of project identity
2. Use its sections to populate `AGENTS.md` placeholders:

| PROJECT_CONTEXT.md section | AGENTS.md section |
|---|---|
| "Identity" | "Project Objective" |
| "Problem" | "Problem" |
| "Goal" | "Goal" |
| "Stack" | "Main Technology" |
| "Architecture" | "Core Architecture" |
| "Directory Layout" | "Project Structure" |
| "Key Commands" | "Development Commands" |
| "Topology" | "Development Commands" (component list) |
| "Notes" | "Key Conventions" |

3. Do not ask the user to re-provide information already in `PROJECT_CONTEXT.md`
4. If `PROJECT_CONTEXT.md` is incomplete, fill gaps from codebase inspection
5. If `PROJECT_CONTEXT.md` does not exist, infer project context from codebase inspection and ask the user for any information that cannot be inferred

### Adaptation, not copying

When you reuse a workflow file from the canonical `engineering-workflow`
repository, **adapt it to this project** — never copy it verbatim:

- replace every `<placeholder>` with a real value; leave none unreplaced;
- delete sections that do not apply (e.g. drop the frontend and visual-QA
  guidance in a repository with no UI);
- replace example commands, directories, and stack names with this project's;
- strip provenance and history that belongs to `engineering-workflow` itself
  (changelogs, issue numbers, ticket references about the library);
- keep the reusable rules that do apply, unchanged in intent.

A copied file carrying another project's commands or history is a defect: it
tells a future agent to run commands this project does not have.

### Agent permissions (mandatory)

When configuring opencode.json, ensure the plan agent has restricted Task tool permissions:

```json
"task": {
  "*": "deny",
  "explore": "allow",
  "reviewer": "allow"
}
```

This prevents plan mode from spawning build agents that bypass read-only restrictions. The plan agent should only spawn `explore` and `reviewer` subagents — both read-only. Implementation is handled by the build agent.

### Branch/PR/Milestone linking (mandatory)

When configuring a project, ensure:
- Branch names include issue numbers: `<type>/<issue-number>-<slug>`
- PRs reference issues: `Closes #<number>`
- Issues are assigned to milestones before work begins
- PRs are assigned to milestones when created
- CODEOWNERS file exists with the project owner

Project-specific configuration may include repository and issue-tracker
information, branch naming conventions, labels, package manager and module
manifest, development commands, GitHub CLI requirements, OpenCode installation or
verification, GitHub Actions requirements, pull request and milestone
conventions, README structure, and project-specific .gitignore entries.

Keep project-specific values separate from the reusable workflow they came from.

If the project uses linting or formatting tools:

- verify the lint and format commands exist (`gofmt`, `go vet`, or equivalent);
- ensure CI runs lint and type-check commands when applicable;
- document available commands in AGENTS.md or instruction files;
- do not add new linting tools unless the project has none and you want one.

If AGENTS.md already exists, preserve valid instructions, remove contradictions
only when clearly justified, merge compatible instructions, do not duplicate
rules, and update it rather than creating another equivalent file. AGENTS.md is
the primary source of project-specific rules for agents — ensure it contains the
workflow rules agents need (branch strategy, CI strategy, milestones, development
commands, environment rules).

If instruction files exist (`.opencode/instructions/*.md`, `PROJECT_CONTEXT.md`):

- verify claims about tests, linters, commands, and file counts match actual state;
- update outdated descriptions rather than leaving contradictions;
- remove references to features or tools that do not exist yet.

If GitHub Actions are used:

- inspect existing workflows before adding new ones; preserve working workflows;
- use branch-based CI: feature branches from `dev`, merge to `dev`, release PR from `dev` to `main`;
- run CI **only** on `dev` → `main` PRs — never on feature branch PRs or push to `dev`;
- require local testing for feature branches;
- do NOT automatically create release PRs or merge to main — the user must explicitly request;
- always return to `dev` after completing any merge;
- document local testing commands in AGENTS.md or README.

If .gitignore exists, preserve valid existing entries and add only
project-specific generated files, local configuration, credentials, caches, or
build output. Do not add broad patterns that hide source or config, and do not
remove an entry unless it is clearly incorrect.

Keep the README simple and readable. Update it when setup, usage, development
commands, authentication, or the contribution workflow changes.

## Phase 3: Identify workflow improvements

Compare the actual project experience with this workflow. Find repeated manual
steps, unnecessary configuration, missing automation, unclear instructions, steps
that caused wasted time, preventable CI failures, tasks that should become
scripts or GitHub Actions, rules in this workflow that are too specific or
ineffective, missing .gitignore entries, documentation that contradicts actual
project state, and dependency-management gaps.

Classify every proposed change as a global improvement, project-specific rule,
automation candidate, documentation improvement, or unnecessary/obsolete rule.

Only promote a project practice to this workflow if it is useful for most future
projects or can be expressed as an optional, conditional rule. Do not promote
project names, repository names, framework-specific commands, business rules,
private URLs, issue IDs, branch names, project-only labels, or assumptions that
are not broadly reusable.

## Phase 4: Apply workflow improvements

Improve this workflow in place only when there is a justified, reusable change.
When you do:

- preserve valid existing instructions; do not duplicate rules;
- keep conditional language ("when applicable", "when the repository uses…");
- do not make parallel-agent execution mandatory;
- when updating workflow files across repos, update templates first, then
  reference files, then configured projects.

## Harvest improvements back to engineering-workflow

When this project produces a reusable improvement to the workflow (a rule, a
check, a script, or a guard that would have prevented the same mistake in any
other project), open an issue in the `engineering-workflow` repository describing:

- **What you observed** — the footgun, friction, or gap
- **What you changed in this project** — the concrete fix
- **Why it's reusable** — why the next project would benefit
- **How you would port it** — which templates/reference files in `engineering-workflow` change, and whether it needs a new validator check
- **Requirement** — mandatory or conditional

Open the issue before closing out this project's work. If you actually port it,
apply the change to `engineering-workflow` in the same spirit: templates first,
reference files second, then validate with `make check`. Reusable improvements
are not optional — capture them, do not leave them siloed here.

## Phase 5: Validate

Before finishing:

- show the files that were changed and the relevant diff;
- verify that no secrets were added;
- verify that no duplicate instructions were created;
- verify that project-specific details did not leak into the reusable workflow;
- verify that the README remains simple;
- verify that setup and CI instructions match the actual project;
- verify that .gitignore changes are project-specific and do not hide important files;
- verify that AGENTS.md and instruction files contain no outdated claims about tests, linters, or commands;
- verify that AGENTS.md contains all necessary workflow rules for agents;
- verify that every copied workflow file was adapted, not copied — no leftover
  placeholders, no other project's commands or history;
- report commands that were run and their results;
- report remaining manual steps.

Do not create a new global workflow prompt. Maintain and improve the existing one.
