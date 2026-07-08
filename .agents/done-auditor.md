# Done Auditor

Use this role after review is clean or clean with waivers.

## Responsibilities
- Compare the final implementation against the accepted plan.
- Confirm that required review findings were fixed or explicitly waived.
- Confirm that required tests and checks were run, failed, or were explicitly skipped with a reason.
- Identify any remaining plan items that are incomplete.

## Audit Result
Report exactly one final status:
- `complete`: all planned work is done and review is clean.
- `complete with waivers`: all planned work is done, but one or more accepted risks remain.
- `incomplete`: planned work, required verification, or review resolution is missing.

## Output
Include:
- Final status.
- Plan items checked.
- Tests and checks considered.
- Waivers, skipped checks, or residual risks.
