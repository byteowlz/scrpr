# Issues

## Open

### [trx-q3wm] Add Jina Reader API backend (P2, epic)
Add Jina Reader API (r.jina.ai) as an extraction backend. Jina Reader converts any URL to clean Markdown without requiring an API key. Simple GET request to https://r.jina.ai/{URL}. Rate limited but free. Good fallback when Tavily credits run out or for simple extraction tasks.

### [trx-v9mn] Add Tavily Extract API backend (P2, epic)
Add Tavily Extract API as an extraction backend option alongside local readability extraction. Tavily provides cloud-based content extraction that handles JavaScript-heavy sites better than local go-readability. Endpoint: POST https://api.tavily.com/extract with Bearer auth. Cost: 1 credit per 5 URLs (basic) or 2 credits per 5 URLs (advanced).

### [trx-7fp0] Document extraction backend selection guide (P3, epic)
Document extraction backend selection guide. Explain when to use readability (default, fast, local), Tavily (better quality, handles JS, API credits), Jina (free, no auth, rate limits). Include comparison table, cost considerations, and example use cases for each backend.

## Closed

- [trx-4t9g] Add native sx to scrpr pipeline integration (closed 2026-02-12)
- [trx-nnba] Complete config file implementation (closed 2026-02-12)
- [trx-mxjm] Implement progress bar for batch operations (closed 2026-02-12)
- [trx-aac3] Add --no-follow-redirects flag (closed 2026-02-12)
- [trx-gtq7] Add continue-on-error flag for pipelines (closed 2026-02-12)
- [trx-pt23] Implement rate limiting between requests (closed 2026-02-12)
- [trx-p7wt] Implement granular exit codes (closed 2026-02-12)
- [trx-n12s] Add quiet/silent mode flag (closed 2026-02-12)
- [trx-9gv8] Complete file output implementation (closed 2026-02-12)
- [trx-akqq] Auto-create config file on first run (closed 2026-02-12)
