# Watchtower Study Generator (Go)

Generador robusto en Go para estudios semanales de La Atalaya con:
- Ingesta manual de `HTML` o `EPUB`
- Persistencia relacional en `SQLite`
- Análisis IA en etapas (A/B/C) con fallback de proveedor
- Flujo de revisión web local (aprobar/rechazar subrayados, editar notas)
- Exportación a formato compacto (`study.html`, `guide.md`, `references.md`)
- **Nuevo**: Investigación con IA usando catálogo oficial JW + EPUBs

## Arquitectura

- `cmd/watchtower`: CLI principal
- `internal/ingest`: ingestión, checksum y estructura por semana
- `internal/parse`: parser normalizado para HTML/EPUB
- `internal/store`: esquema SQLite + migraciones + repositorios
- `internal/llm`: OpenAI, Gemini y endpoint OpenAI-compatible local
- `internal/analysis`: pipeline prompt-engineering por etapas
- `internal/web`: mini web de revisión y edición
- `internal/render`: exportación final
- `internal/catalog`: catálogo oficial JW (manifest v5, GETPUBMEDIALINKS)
- `internal/epub`: descarga y extracción de EPUBs oficiales
- `internal/research`: generación de bosquejos con citas obligatorias

## Estructura de salida

Cada semana se guarda en:

```text
studies/YYYY-WNN/
  source/
  data/study.db
  outputs/study.html
  outputs/guide.md
  outputs/references.md
```

## Configuración

Archivo: `watchtower.yaml`

Variables compatibles:
- `OPENAI_API_KEY`
- `GEMINI_API_KEY`
- `LOCAL_LLM_URL`
- `LOCAL_LLM_MODEL`
- `LOCAL_LLM_ENABLED`

## Comandos

```bash
# 1) Ingesta
# (entrada manual html/epub)
go run ./cmd/watchtower ingest --week 2026-W14 --input studies/2026-W14/article.html

# 2) Análisis IA (modo por defecto: balanced)
go run ./cmd/watchtower analyze --week 2026-W14 --provider auto --mode balanced

# 3) Revisión interactiva (web local)
go run ./cmd/watchtower review --week 2026-W14
# abre http://127.0.0.1:8088

# 4) Exportación
go run ./cmd/watchtower export --week 2026-W14

# Pipeline completo
go run ./cmd/watchtower run --week 2026-W14 --input studies/2026-W14/article.html --provider auto --mode balanced
```

### Investigación con IA (nuevo)

Usa el catálogo oficial JW para generar bosquejos con **citas obligatorias**:

```bash
# Descargar catálogo oficial (333K+ publicaciones)
go run ./cmd/watchtower catalog refresh

# Buscar publicaciones
go run ./cmd/watchtower catalog search --symbol w26 --lang S
go run ./cmd/watchtower catalog search --year 2026 --lang S

# Generar bosquejo de investigación
go run ./cmd/watchtower research --pub w --issue 202601 --topic "fe y persistencia"
go run ./cmd/watchtower research --pub w --issue 202601 --topic "fe" --output outline.md
```

**Regla core**: "Sin cita = excluido". Cada afirmación doctrinal debe tener fuente verificable.

## Desarrollo

```bash
go mod tidy
go test ./...
```

## Estado

Esta base ya incluye:
- Esquema relacional completo con trazas de prompts/respuestas
- Validación de subrayados contra substring real del párrafo
- Fallback automático de proveedores (`OpenAI -> Gemini -> local`)
- Regeneración por párrafo desde la mini web
- **Catálogo oficial JW** (API v5, 333K+ publicaciones, 966 idiomas)
- **Descarga de EPUBs** oficiales via GETPUBMEDIALINKS
- **Generación de bosquejos** con filtro "sin cita = excluido"

No incluye en v1:
- Soporte `JWPub` (contenido en formato MEPS comprimido)
