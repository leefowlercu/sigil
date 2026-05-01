package harness

import "testing"

func TestNormalizeFinalAnswerTrimsBoundaryWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain",
			input: "answer",
			want:  "answer",
		},
		{
			name:  "boundary whitespace",
			input: "\n  answer  \n\t",
			want:  "answer",
		},
		{
			name:  "internal whitespace",
			input: "alpha\n  beta  \ngamma",
			want:  "alpha\n  beta  \ngamma",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeFinalAnswer(test.input); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestExtractMarkedFinalAnswerCandidate(t *testing.T) {
	stdout := "debug\nFINAL_ANSWER_START\nexact answer\nwith lines\nFINAL_ANSWER_END\nmore debug"

	candidate, ok := extractMarkedFinalAnswerCandidate(stdout)
	if !ok {
		t.Fatal("expected marked candidate")
	}
	expected := "exact answer\nwith lines"
	if candidate != expected {
		t.Fatalf("expected candidate %q, got %q", expected, candidate)
	}
}

func TestExtractMarkedFinalAnswerCandidateRequiresCanonicalMarkers(t *testing.T) {
	stdout := "ASSISTANT_TEXT_START\nsource span only\nASSISTANT_TEXT_END"

	if candidate, ok := extractMarkedFinalAnswerCandidate(stdout); ok {
		t.Fatalf("expected no candidate, got %q", candidate)
	}
}
