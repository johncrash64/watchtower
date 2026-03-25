package research

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"watchtower/internal/epub"
	"watchtower/internal/llm"
)

type mockFetcher struct {
	path string
	err  error
}

func (m mockFetcher) Fetch(_ context.Context, _, _, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.path, nil
}

type mockExtractor struct {
	content *epub.EPUBContent
	err     error
}

func (m mockExtractor) Extract(_ string) (*epub.EPUBContent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.content, nil
}

type mockLLMClient struct {
	name string
	text string
	err  error
}

func (m mockLLMClient) Name() string  { return m.name }
func (m mockLLMClient) Model() string { return "mock" }
func (m mockLLMClient) Generate(_ context.Context, _ llm.Request) (llm.Response, error) {
	if m.err != nil {
		return llm.Response{}, m.err
	}
	return llm.Response{Text: m.text}, nil
}

func TestResearch_FullFlowWithMocks(t *testing.T) {
	epubPath := createTestEPUB(t, `<html><body><p><a epub:type="noteref" href="#citation1">MAT. 5:3</a></p></body></html>`)

	r := newResearcherWithDeps(
		mockFetcher{path: epubPath},
		mockExtractor{content: &epub.EPUBContent{Articles: []epub.EPUBArticle{{Title: "Artículo de prueba"}}}},
		[]llm.Client{
			mockLLMClient{name: "mock", text: strings.Join([]string{
				"## Bosquejo",
				"",
				"Punto válido [EPUB: MAT. 5:3]",
				"",
				"Punto inválido [EPUB: ref inventada]",
			}, "\n")},
		},
		"balanced",
	)

	out, err := r.Research(context.Background(), "Fe", "w", "202601")
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}

	if !strings.Contains(out.OutlineText, "## Bosquejo") || !strings.Contains(out.OutlineText, "Punto válido") {
		t.Fatalf("OutlineText should include generated outline and valid claim, got %q", out.OutlineText)
	}
	if strings.Contains(out.OutlineText, "Punto inválido") {
		t.Fatalf("OutlineText should not include filtered invalid claim")
	}
	if len(out.CitationsUsed) != 1 || out.CitationsUsed[0] != "MAT. 5:3" {
		t.Fatalf("CitationsUsed = %#v, want []string{\"MAT. 5:3\"}", out.CitationsUsed)
	}
	if len(out.FilteredClaims) != 1 || !strings.Contains(out.FilteredClaims[0], "Punto inválido") {
		t.Fatalf("FilteredClaims = %#v, want one removed invalid claim", out.FilteredClaims)
	}
}

func TestResearch_ErrorHandlingStages(t *testing.T) {
	epubPath := createTestEPUB(t, `<html><body><p><a epub:type="noteref" href="#citation1">MAT. 5:3</a></p></body></html>`)
	baseContent := &epub.EPUBContent{Articles: []epub.EPUBArticle{{Title: "Artículo"}}}

	tests := []struct {
		name    string
		r       *Researcher
		wantErr error
	}{
		{
			name: "fetch error",
			r: newResearcherWithDeps(
				mockFetcher{err: errors.New("network down")},
				mockExtractor{content: baseContent},
				[]llm.Client{mockLLMClient{name: "mock", text: "ok [EPUB: MAT. 5:3]"}},
				"balanced",
			),
			wantErr: errors.New("fetch EPUB"),
		},
		{
			name: "extract error",
			r: newResearcherWithDeps(
				mockFetcher{path: epubPath},
				mockExtractor{err: errors.New("bad zip")},
				[]llm.Client{mockLLMClient{name: "mock", text: "ok [EPUB: MAT. 5:3]"}},
				"balanced",
			),
			wantErr: errors.New("extract EPUB content"),
		},
		{
			name: "llm error",
			r: newResearcherWithDeps(
				mockFetcher{path: epubPath},
				mockExtractor{content: baseContent},
				[]llm.Client{mockLLMClient{name: "mock", err: errors.New("llm timeout")}},
				"balanced",
			),
			wantErr: errors.New("generate grounded outline"),
		},
		{
			name: "all claims filtered",
			r: newResearcherWithDeps(
				mockFetcher{path: epubPath},
				mockExtractor{content: baseContent},
				[]llm.Client{mockLLMClient{name: "mock", text: "Afirmación sin cita"}},
				"balanced",
			),
			wantErr: ErrAllClaimsFiltered,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.r.Research(context.Background(), "Fe", "w", "202601")
			if err == nil {
				t.Fatalf("expected error %v, got nil", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func createTestEPUB(t *testing.T, xhtml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "w_s_202601.epub")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test EPUB: %v", err)
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	entry, err := zw.Create("OEBPS/article1.xhtml")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write([]byte(xhtml)); err != nil {
		t.Fatalf("write xhtml entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return path
}
