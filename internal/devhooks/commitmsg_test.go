package devhooks

import (
	"strings"
	"testing"
)

func TestValidateCommitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		wantErr string
	}{
		{
			name:    "accepts conventional commit",
			message: "feat(cli): add docs validation\n\nbody\n",
		},
		{
			name:    "accepts breaking change marker",
			message: "refactor(cli)!: slim bootstrap layer\n",
		},
		{
			name:    "accepts merge subject",
			message: "Merge branch 'feature/test'\n",
		},
		{
			name:    "accepts revert subject",
			message: "Revert \"feat(cli): add docs validation\"\n",
		},
		{
			name:    "skips comments and blank lines",
			message: "\n# Please enter the commit message\n\nfix(hooks): reject empty subject\n",
		},
		{
			name:    "rejects empty subject",
			message: "\n# comment only\n",
			wantErr: "commit message subject is empty",
		},
		{
			name:    "rejects non conventional subject",
			message: "update CLI behavior\n",
			wantErr: "Invalid commit message.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateCommitMessage(strings.NewReader(test.message))
			if test.wantErr == "" && err != nil {
				t.Fatalf("ValidateCommitMessage() error = %v", err)
			}

			if test.wantErr != "" {
				if err == nil {
					t.Fatal("ValidateCommitMessage() error = nil, want non-nil")
				}

				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ValidateCommitMessage() error = %q, want substring %q", err.Error(), test.wantErr)
				}
			}
		})
	}
}
