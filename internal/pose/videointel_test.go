package pose

import "testing"

func TestNormalizeLandmarkName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nose", "nose"},
		{"LEFT_SHOULDER", "left_shoulder"},
		{"Right_Wrist", "right_wrist"},
		{"LEFT_EYE", "left_eye"},
		{"unknown_landmark", "unknown_landmark"},
	}
	for _, tt := range tests {
		got := normalizeLandmarkName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLandmarkName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
