# Project Architecture Rules (Non-Obvious Only)

- Single-file architecture by design - [`build_study_package.py`](build_study_package.py) contains all parsing, transformation, and generation logic
- Data flow: MHTML → BeautifulSoup parse → article clean → notes injection → HTML/MD output (linear pipeline)
- `study_notes.json` is the only configuration source - no external config files supported
- Output generation is idempotent - running multiple times produces identical results
- No database or state persistence beyond `.bible_tooltips_cache.json` cache file
- WOL API calls are optional and controlled by single flag - system works offline without them
