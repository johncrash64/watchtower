# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Project Overview

Robust Go-based generator for weekly Watchtower study materials. Supports manual HTML/EPUB ingestion, AI analysis using multiple providers, interactive local web review, and standardized Markdown/HTML export.

## Commands

```bash
# Ingest
go run ./cmd/watchtower ingest --week YYYY-WNN --input studies/YYYY-WNN/article.html

# Analyze
go run ./cmd/watchtower analyze --week YYYY-WNN --provider auto --mode balanced

# Review (Web App on :8088)
go run ./cmd/watchtower review --week YYYY-WNN

# Export
go run ./cmd/watchtower export --week YYYY-WNN

# All at once
go run ./cmd/watchtower run --week YYYY-WNN --input studies/YYYY-WNN/article.html --provider auto
```

## Architecture

- **CLI App:** `cmd/watchtower`
- **Internal Modules:** `ingest`, `parse`, `store` (SQLite), `llm`, `analysis`, `web`, `render`.
- **Storage:** Relational schema in SQLite (`study.db`).

## Key Conventions

- Output is strictly organized by week: `studies/YYYY-WNN/outputs/`.
- `watchtower.yaml` configures providers and variables.
- AI uses a fallback mechanism (`OpenAI -> Gemini -> Local`).
- Legacy Python processing is completely removed.
