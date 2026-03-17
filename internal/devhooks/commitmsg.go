package devhooks

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var conventionalCommitSubject = regexp.MustCompile(`^(feat|fix|docs|chore|ci|refactor|test|revert)(\([a-z0-9._/-]+\))?(!)?: .+`)

const conventionalCommitHelp = `Invalid commit message.
Expected Conventional Commits format:
  <type>(optional-scope): <description>

Examples:
  feat(link): add retry logic for transient failures
  fix(auth): handle expired token refresh
  chore(ci): optimize pre-push checks`

// ValidateCommitMessage checks the commit subject against the repo policy.
func ValidateCommitMessage(rdr io.Reader) error {
	subject, err := readCommitSubject(rdr)
	if err != nil {
		return err
	}

	if subject == "" {
		return fmt.Errorf("commit message subject is empty")
	}

	if strings.HasPrefix(subject, "Merge ") || strings.HasPrefix(subject, "Revert ") {
		return nil
	}

	if conventionalCommitSubject.MatchString(subject) {
		return nil
	}

	return fmt.Errorf("%s", conventionalCommitHelp)
}

func readCommitSubject(rdr io.Reader) (string, error) {
	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		return line, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read commit message: %w", err)
	}

	return "", nil
}
