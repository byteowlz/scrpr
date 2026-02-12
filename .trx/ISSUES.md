# Issues

## Open

### [trx-9gv8] Complete file output implementation (P0, epic)
File output exists as TODO comment instead of implementation. Need to implement -o for single file and -o dir for multiple files with auto-naming. Support both text and markdown formats. Handle file naming collisions properly.

### [trx-akqq] Auto-create config file on first run (P0, epic)
# Problem
Unlike sx, scrpr does not auto-create config.toml on first run.

# Proposed Solution
- Check for config on startup
...


### [trx-p7wt] Implement granular exit codes (P1, epic)
# Problem
All failures exit with code 1. Can't distinguish between different failure types in scripts.

# Proposed Solution
Define exit codes:
...


### [trx-n12s] Add quiet/silent mode flag (P1, epic)
# Problem
No --quiet flag to suppress error messages for script usage.

# Proposed Solution
- Add -q/--quiet flag
...


### [trx-nnba] Complete config file implementation (P2, epic)
Config file has rate_limit and parallel sections but they're not fully implemented.

Need to implement:
- rate_limit.delay between requests
- parallel.max_concurrency 
...


### [trx-pt23] Implement rate limiting between requests (P2, epic)
No rate limiting between requests. When piping multiple URLs, could hammer target sites.

Solution: Implement the existing rate_limit.delay config option from config.toml. Add --delay flag to CLI. Default to 0.5s between requests. Show warning if processing many URLs without delay.

### [trx-gtq7] Add continue-on-error flag for pipelines (P2, epic)
When processing multiple URLs via pipe, failures break the entire pipeline. One bad URL stops processing.

Solution: Add --continue-on-error flag. When enabled, log error but continue processing remaining URLs. Return non-zero exit code at end if any errors occurred. Default should be true for piped input.

### [trx-aac3] Add --no-follow-redirects flag (P2, epic)
No option to disable redirect following. Currently hardcoded to follow redirects.

Solution: Add --no-follow-redirects flag to control redirect behavior. Default should remain true for backward compatibility. Document in README.

### [trx-mxjm] Implement progress bar for batch operations (P2, epic)
The --progress flag is documented in help but not actually implemented. Need to add progress bar for batch URL processing. Consider using schollz/progressbar or similar library. Show progress when processing multiple URLs with -f or piped input.

### [trx-4t9g] Add native sx to scrpr pipeline integration (P3, epic)
No native integration between sx and scrpr. Users need to manually extract URLs.

Solutions to consider:
1. Add --pipe flag to sx that outputs URLs formatted for scrpr
2. Add --from-json flag to scrpr to read sx JSON output
...


