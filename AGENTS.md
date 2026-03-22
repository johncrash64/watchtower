# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Project Overview

Study package generator for Watchtower articles. Parses MHTML from WOL (Watchtower Online Library) and generates HTML + markdown study materials.

## Commands

```bash
python build_study_package.py# Generates all outputs
```

## Architecture

- **Input:** MHTML file (downloaded from WOL) + `study_notes.json`
- **Output:** 1 HTML file + 7 markdown files (00-06 prefix)
- **Main script:** [`build_study_package.py`](build_study_package.py) - single 2600+ line file containing all logic

## Key Conventions

- MHTML source file must exist in project root (script auto-detects `.mhtml` files)
- Output filenames are fixed in `OUTPUT_FILES` dict - do not rename
- Bible book aliases in `BIBLE_BOOK_ALIASES` use Spanish abbreviations
- Remote bible tooltips disabled by default (`ENABLE_REMOTE_BIBLE_TOOLTIPS = False`)
- Cache file `.bible_tooltips_cache.json` speeds up subsequent runs

## Code Style

- Type hints used throughout (`dict[str, Any]` syntax)
- Path operations via `pathlib.Path`
- CSS embedded as multiline string in `LOCAL_CSS`
- No external config files - all constants at module level
