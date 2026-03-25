package research

import (
	"reflect"
	"testing"
)

func TestFilterUncitedClaims(t *testing.T) {
	valid := []string{"w 202601 p7", "MAT. 5:3"}

	tests := []struct {
		name         string
		text         string
		valid        []string
		wantFiltered string
		wantRemoved  []string
	}{
		{
			name:         "text with valid citation passes through",
			text:         "Jesús predijo la destrucción [EPUB: w 202601 p7]",
			valid:        valid,
			wantFiltered: "Jesús predijo la destrucción [EPUB: w 202601 p7]",
			wantRemoved:  []string{},
		},
		{
			name:         "text with invalid citation is filtered",
			text:         "Afirmación sin respaldo real [EPUB: w 202601 p999]",
			valid:        valid,
			wantFiltered: "",
			wantRemoved:  []string{"Afirmación sin respaldo real [EPUB: w 202601 p999]"},
		},
		{
			name:         "text without citations is filtered",
			text:         "El templo fue destruido en 70 d.C.",
			valid:        valid,
			wantFiltered: "",
			wantRemoved:  []string{"El templo fue destruido en 70 d.C."},
		},
		{
			name: "mixed text keeps valid blocks and headings",
			text: "## Título\n\n" +
				"Afirmación válida [EPUB: w 202601 p7]\n\n" +
				"Afirmación inválida [EPUB: w 202601 p999]\n\n" +
				"Afirmación sin cita",
			valid:        valid,
			wantFiltered: "## Título\n\nAfirmación válida [EPUB: w 202601 p7]",
			wantRemoved: []string{
				"Afirmación inválida [EPUB: w 202601 p999]",
				"Afirmación sin cita",
			},
		},
		{
			name:         "empty text edge case",
			text:         "   ",
			valid:        valid,
			wantFiltered: "",
			wantRemoved:  nil,
		},
		{
			name:         "all valid edge case",
			text:         "Una [EPUB: w 202601 p7]\n\nDos [EPUB: MAT. 5:3]",
			valid:        valid,
			wantFiltered: "Una [EPUB: w 202601 p7]\n\nDos [EPUB: MAT. 5:3]",
			wantRemoved:  []string{},
		},
		{
			name:         "all invalid edge case",
			text:         "Una [EPUB: ref inventada]\n\nDos sin cita",
			valid:        valid,
			wantFiltered: "",
			wantRemoved: []string{
				"Una [EPUB: ref inventada]",
				"Dos sin cita",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFiltered, gotRemoved := FilterUncitedClaims(tt.text, tt.valid)
			if gotFiltered != tt.wantFiltered {
				t.Fatalf("filteredText = %q, want %q", gotFiltered, tt.wantFiltered)
			}
			if !reflect.DeepEqual(gotRemoved, tt.wantRemoved) {
				t.Fatalf("removedClaims = %#v, want %#v", gotRemoved, tt.wantRemoved)
			}
		})
	}
}
