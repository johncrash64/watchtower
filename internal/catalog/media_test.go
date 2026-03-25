package catalog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetEPUBURL(t *testing.T) {
	tests := []struct {
		name         string
		pub          string
		issue        string
		lang         string
		status       int
		body         string
		wantURL      string
		wantSize     int64
		wantChecksum string
		wantMimeType string
		wantErr      error
		assertQuery  func(t *testing.T, req *http.Request)
	}{
		{
			name:         "parse valid JSON response with EPUB URL",
			pub:          "w",
			issue:        "202601",
			lang:         "S",
			status:       http.StatusOK,
			body:         `{"files":{"S":{"EPUB":[{"file":{"url":"https://cdn.example/w.epub","filesize":"12345","checksum":"abc123","mimetype":"application/epub+zip"}}]}}}`,
			wantURL:      "https://cdn.example/w.epub",
			wantSize:     12345,
			wantChecksum: "abc123",
			wantMimeType: "application/epub+zip",
			assertQuery: func(t *testing.T, req *http.Request) {
				t.Helper()
				q := req.URL.Query()
				if q.Get("output") != "json" || q.Get("fileformat") != "EPUB" {
					t.Fatalf("unexpected fixed params: %s", req.URL.RawQuery)
				}
				if q.Get("pub") != "w" || q.Get("issue") != "202601" || q.Get("langwritten") != "S" {
					t.Fatalf("unexpected query params: %s", req.URL.RawQuery)
				}
			},
		},
		{
			name:    "handle missing EPUB format in response",
			pub:     "w",
			issue:   "202601",
			lang:    "S",
			status:  http.StatusOK,
			body:    `{"files":{"S":{"MP3":[]}}}`,
			wantErr: ErrEPUBURLNotFound,
		},
		{
			name:    "handle empty files object",
			pub:     "w",
			issue:   "202601",
			lang:    "S",
			status:  http.StatusOK,
			body:    `{"files":{}}`,
			wantErr: ErrEPUBURLNotFound,
		},
		{
			name:    "handle invalid JSON",
			pub:     "w",
			issue:   "202601",
			lang:    "S",
			status:  http.StatusOK,
			body:    `{invalid json`,
			wantErr: errors.New("decode media response"),
		},
		{
			name:    "construct URL with default lang and normalized watchtower pub",
			pub:     "w202601",
			issue:   "",
			lang:    "",
			status:  http.StatusOK,
			body:    `{"files":{"S":{"EPUB":[{"file":{"url":"https://cdn.example/current.epub"}}]}}}`,
			wantURL: "https://cdn.example/current.epub",
			assertQuery: func(t *testing.T, req *http.Request) {
				t.Helper()
				q := req.URL.Query()
				if q.Get("pub") != "w" {
					t.Fatalf("pub should be normalized to w, got %q", q.Get("pub"))
				}
				if q.Get("langwritten") != "S" {
					t.Fatalf("default lang should be S, got %q", q.Get("langwritten"))
				}
				if q.Get("issue") != "" {
					t.Fatalf("issue should be omitted when empty, got %q", q.Get("issue"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMediaClient()
			client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == "" {
					t.Fatalf("request URL should not be empty")
				}
				if tt.assertQuery != nil {
					tt.assertQuery(t, req)
				}
				return &http.Response{
					StatusCode: tt.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    req,
				}, nil
			})}

			got, err := client.GetEPUBURL(context.Background(), tt.pub, tt.issue, tt.lang)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetEPUBURL() error = %v", err)
			}
			if got == nil {
				t.Fatalf("GetEPUBURL() returned nil response")
			}
			if got.URL != tt.wantURL {
				t.Fatalf("URL = %q, want %q", got.URL, tt.wantURL)
			}
			if got.FileSize != tt.wantSize {
				t.Fatalf("FileSize = %d, want %d", got.FileSize, tt.wantSize)
			}
			if got.Checksum != tt.wantChecksum {
				t.Fatalf("Checksum = %q, want %q", got.Checksum, tt.wantChecksum)
			}
			if got.MimeType != tt.wantMimeType {
				t.Fatalf("MimeType = %q, want %q", got.MimeType, tt.wantMimeType)
			}
		})
	}
}
