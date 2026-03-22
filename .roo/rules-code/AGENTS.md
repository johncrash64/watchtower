# Project Coding Rules (Non-Obvious Only)

- All logic resides in single file [`build_study_package.py`](build_study_package.py) - do not split into modules
- Output filenames fixed in `OUTPUT_FILES` dict at line24 - changing names breaks cross-references
- Bible book aliases in `BIBLE_BOOK_ALIASES` use Spanish abbreviations (e.g., "rom"→"romanos", "sal"→"salmos")
- CSS embedded as `LOCAL_CSS` multiline string (line72) - no external stylesheet files
- `ENABLE_REMOTE_BIBLE_TOOLTIPS = False` disables WOL API calls - set `True` only if network available
- Cache file `.bible_tooltips_cache.json` stores API responses - delete to force refresh
