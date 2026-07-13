# Agent Development Workflow

This repo uses a staged agent workflow for feature work, behavioral changes, and non-trivial fixes. The goal is to keep planning, coding, review, and final completion checks separate enough that each stage can catch different failure modes.

## Roles

Use the role prompts in `.agents/`:

- Planner: creates the accepted plan and does not edit files.
- Coding agent: implements the accepted plan, updates tests, and records verification.
- Review agent: reviews for defects, regressions, missing tests, and plan drift.
- Done auditor: confirms that the final work matches the accepted plan.

## Model Selection

- Planning always uses the latest available model because planning owns product interpretation, architecture, risk identification, and the decision-complete implementation contract.
- Coding, review, and done-audit agents should use GPT-5.5 when the assignment is well-scoped and its complexity and risk are appropriate for that model.
- Use the latest available model instead of GPT-5.5 for unusually complex or ambiguous cross-system changes, security- or data-loss-sensitive work, difficult concurrency or migration work, and reviews where subtle correctness risks dominate cost or speed.
- If a requested model is unavailable or usage-limited, use the strongest available fallback and record the substitution in the task record. Model availability must not silently skip a required workflow stage.

## Workflow

1. A planner using the latest available model reads the repo, resolves discoverable facts, asks only necessary questions, and produces a decision-complete plan.
2. Coding agent implements the accepted plan, keeps scope tight, updates tests, and records any deviations.
3. Review agent reviews the implementation against the plan and repo guidelines.
4. Coding agent fixes each finding or records an explicit waiver with the risk and reason.
5. Review repeats until the status is `clean` or `clean with waivers`.
6. Done auditor checks the final state against the plan and reports `complete`, `complete with waivers`, or `incomplete`.

## Required Handoff Artifacts

Each task should keep a task record using `.agents/task-template.md`. The record must include:

- Accepted plan.
- Implementation summary.
- Files changed.
- Tests and checks run.
- Review findings.
- Fixes or explicit waivers for every finding.
- Final audit status.

## Review Standard

Every review finding must be fixed or explicitly waived before the task is considered done. Waivers are allowed only when they name the remaining risk and explain why accepting it is appropriate for the task.

Review output should lead with findings and include file and line references when applicable. Summaries are secondary.

## Completion Standard

A task is done only when:

- The accepted plan has been implemented or deviations are documented and accepted.
- Required tests or checks have run, or skipped checks have explicit reasons.
- Review is `clean` or `clean with waivers`.
- The done audit reports `complete` or `complete with waivers`.
