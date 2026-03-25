package epub

import "testing"

func TestExtractCitationsFromHTML(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantLen int
		want    []CitationRef
	}{
		{
			name:    "extract single citation",
			html:    `<p><a epub:type="noteref" href="#citation1">MAT. 5:3</a></p>`,
			wantLen: 1,
			want:    []CitationRef{{Key: "MAT. 5:3", RefID: "citation1"}},
		},
		{
			name: "extract multiple citations from one paragraph",
			html: `<p>
				<a epub:type="noteref" href="#citation1">MAT. 5:3</a>
				<a epub:type="noteref" href="#citation2">Mat. 15:21-28</a>
				<a epub:type="noteref" href="#citation3">Juan 6:66-68</a>
			</p>`,
			wantLen: 3,
			want: []CitationRef{
				{Key: "MAT. 5:3", RefID: "citation1"},
				{Key: "Mat. 15:21-28", RefID: "citation2"},
				{Key: "Juan 6:66-68", RefID: "citation3"},
			},
		},
		{
			name: "missing attributes are ignored gracefully",
			html: `<p>
				<a epub:type="noteref">MAT. 5:3</a>
				<a epub:type="noteref" href="#citation2"></a>
				<a href="#citation3">Juan 6:66-68</a>
			</p>`,
			wantLen: 0,
		},
		{
			name:    "no citations returns empty slice",
			html:    `<p>Sin referencias en este párrafo</p>`,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCitationsFromHTML(tt.html)

			if len(got) != tt.wantLen {
				t.Fatalf("len(ExtractCitationsFromHTML()) = %d, want %d", len(got), tt.wantLen)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("citation[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractScriptureCitationsFromHTML_PopulatesStructFields(t *testing.T) {
	html := `<p>
		<a epub:type="noteref" href="#citation1">MAT. 5:3</a>
		<a epub:type="noteref" href="#citation2">Mat. 15:21-28</a>
		<a epub:type="noteref" href="#citation3">Juan 6:66-68</a>
	</p>`

	got := ExtractScriptureCitationsFromHTML(html)
	if len(got) != 3 {
		t.Fatalf("len(ExtractScriptureCitationsFromHTML()) = %d, want 3", len(got))
	}

	want := []ScriptureCitation{
		{Book: "Mateo", Chapter: 5, Verses: "3", OriginalText: "MAT. 5:3"},
		{Book: "Mateo", Chapter: 15, Verses: "21-28", OriginalText: "Mat. 15:21-28"},
		{Book: "Juan", Chapter: 6, Verses: "66-68", OriginalText: "Juan 6:66-68"},
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scripture[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
