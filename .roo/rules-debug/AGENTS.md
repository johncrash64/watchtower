# Project Debug Rules (Non-Obvious Only)

- Script fails silently if no `.mhtml` file exists in project root - check file exists first
- `study_notes.json` must have matching paragraph counts with MHTML source or validation fails
- MHTML parsing uses `email.parser.BytesParser` - encoding issues may occur with non-UTF8 sources
- Delete `.bible_tooltips_cache.json` if scripture tooltips show stale/incorrect content
- Set `ENABLE_REMOTE_BIBLE_TOOLTIPS = True` and ensure network access if tooltips need live WOL data
- BeautifulSoup warnings about malformed HTML are normal - WOL source HTML is intentionally cleaned
