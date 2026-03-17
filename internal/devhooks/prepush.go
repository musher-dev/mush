package devhooks

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

const zeroSHA = "0000000000000000000000000000000000000000"

type taskRunner func(context.Context, string, ...string) error

type gitDiffRunner func(context.Context, string, string) ([]string, error)

type pushRef struct {
	localRef  string
	localSHA  string
	remoteRef string
	remoteSHA string
}

type changeSet struct {
	goFiles       []string
	shellFiles    []string
	workflowFiles []string
	lintPackages  []string
	testPackages  []string
	modChanged    bool
	installTests  bool
}

// RunPrePush applies the repo's pre-push checks using git diff data from stdin.
func RunPrePush(ctx context.Context, stdin io.Reader, runTask taskRunner, gitDiff gitDiffRunner) error {
	refs, err := parsePushRefs(stdin)
	if err != nil {
		return err
	}

	changedFiles, fallbackFull, err := collectChangedFiles(ctx, refs, gitDiff)
	if err != nil {
		return err
	}

	if fallbackFull {
		return runFullPrePush(ctx, runTask)
	}

	if len(changedFiles) == 0 {
		return nil
	}

	changes := classifyChanges(changedFiles)
	if err := runTargetedChecks(ctx, runTask, &changes); err != nil {
		return err
	}

	return runTargetedTests(ctx, runTask, &changes)
}

func runFullPrePush(ctx context.Context, runTask taskRunner) error {
	for _, taskName := range []string{"check:lint", "check:policy", "check:test"} {
		if err := runTask(ctx, taskName); err != nil {
			return err
		}
	}

	return nil
}

func parsePushRefs(stdin io.Reader) ([]pushRef, error) {
	scanner := bufio.NewScanner(stdin)
	refs := make([]pushRef, 0)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 4 {
			return nil, fmt.Errorf("invalid pre-push ref line %q", line)
		}

		refs = append(refs, pushRef{
			localRef:  fields[0],
			localSHA:  fields[1],
			remoteRef: fields[2],
			remoteSHA: fields[3],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read pre-push refs: %w", err)
	}

	return refs, nil
}

func collectChangedFiles(ctx context.Context, refs []pushRef, gitDiff gitDiffRunner) (files []string, fallbackFull bool, err error) {
	changedSet := make(map[string]struct{})

	for _, ref := range refs {
		if ref.localSHA == zeroSHA {
			continue
		}

		if ref.remoteSHA == zeroSHA {
			fallbackFull = true
			continue
		}

		changed, err := gitDiff(ctx, ref.remoteSHA, ref.localSHA)
		if err != nil {
			return nil, false, err
		}

		for _, file := range changed {
			if file == "" {
				continue
			}

			changedSet[file] = struct{}{}
		}
	}

	files = make([]string, 0, len(changedSet))
	for file := range changedSet {
		files = append(files, file)
	}

	slices.Sort(files)

	return files, fallbackFull, nil
}

func runTargetedChecks(ctx context.Context, runTask taskRunner, changes *changeSet) error {
	for _, step := range buildTargetedSteps(changes) {
		if err := runTask(ctx, step.name, step.args...); err != nil {
			return err
		}
	}

	return nil
}

func runTargetedTests(ctx context.Context, runTask taskRunner, changes *changeSet) error {
	if changes.modChanged {
		return runTask(ctx, "check:test:packages", "--", "./...")
	}

	if len(changes.testPackages) > 0 {
		if err := runTask(ctx, "check:test:packages", append([]string{"--"}, changes.testPackages...)...); err != nil {
			return err
		}
	}

	return nil
}

type taskInvocation struct {
	name string
	args []string
}

func buildTargetedSteps(changes *changeSet) []taskInvocation {
	steps := make([]taskInvocation, 0, 6)

	if len(changes.goFiles) > 0 {
		steps = append(steps, taskInvocation{
			name: "check:fmt:files",
			args: append([]string{"--"}, changes.goFiles...),
		})
	}

	if len(changes.lintPackages) > 0 {
		steps = append(steps, taskInvocation{
			name: "check:lint:files",
			args: append([]string{"--"}, changes.lintPackages...),
		})
	}

	if len(changes.goFiles) > 0 || changes.modChanged {
		steps = append(steps, taskInvocation{name: "check:policy"})
	}

	if changes.modChanged {
		steps = append(steps,
			taskInvocation{name: "check:mod"},
			taskInvocation{name: "check:vuln"},
		)
	}

	if len(changes.shellFiles) > 0 {
		steps = append(steps, taskInvocation{
			name: "check:shell:files",
			args: append([]string{"--"}, changes.shellFiles...),
		})
	}

	if len(changes.workflowFiles) > 0 {
		steps = append(steps, taskInvocation{
			name: "check:workflow:files",
			args: append([]string{"--"}, changes.workflowFiles...),
		})
	}

	if changes.installTests {
		steps = append(steps, taskInvocation{name: "check:install"})
	}

	return steps
}

func classifyChanges(changedFiles []string) changeSet {
	goFiles := make([]string, 0)
	shellFiles := make([]string, 0)
	workflowFiles := make([]string, 0)
	lintPackages := make(map[string]struct{})
	testPackages := make(map[string]struct{})
	modChanged := false
	installTests := false

	for _, file := range changedFiles {
		switch {
		case strings.HasSuffix(file, ".go"):
			goFiles = append(goFiles, file)
			lintPackages[lintPackageForFile(file)] = struct{}{}
			testPackages[testPackageForFile(file)] = struct{}{}
		case file == "go.mod" || file == "go.sum":
			modChanged = true
		case strings.HasSuffix(file, ".sh"):
			shellFiles = append(shellFiles, file)
			if file == "install.sh" {
				installTests = true
			}
		case strings.HasPrefix(file, ".github/workflows/") &&
			(strings.HasSuffix(file, ".yml") || strings.HasSuffix(file, ".yaml")):
			workflowFiles = append(workflowFiles, file)
		case strings.HasPrefix(file, "test/install/"):
			installTests = true
		}
	}

	slices.Sort(goFiles)
	slices.Sort(shellFiles)
	slices.Sort(workflowFiles)

	return changeSet{
		goFiles:       goFiles,
		shellFiles:    shellFiles,
		workflowFiles: workflowFiles,
		lintPackages:  sortedKeys(lintPackages),
		testPackages:  sortedKeys(testPackages),
		modChanged:    modChanged,
		installTests:  installTests,
	}
}

func lintPackageForFile(path string) string {
	if !strings.Contains(path, "/") {
		return "./"
	}

	lastSlash := strings.LastIndex(path, "/")

	return "./" + path[:lastSlash] + "/..."
}

func testPackageForFile(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		return "./"
	}

	return "./" + path[:lastSlash]
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}

	slices.Sort(keys)

	return keys
}
