// Command compatibility compares the public Go API with a Git baseline.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultModulePath        = "github.com/portpowered/go-roborock"
	defaultPublicPackages    = "pkg/roborock"
	apiDiffTool              = "golang.org/x/exp/cmd/apidiff@v0.0.0-20260908205506-85c1c2202aba"
	previousRelease          = "previous-release"
	policyReport             = "report"
	policyRelease            = "release"
	versionCaptureCount      = 4
	toolDirectoryPermissions = 0o755
	summaryFilePermissions   = 0o600
)

var (
	stableTagPattern  = regexp.MustCompile("^v([0-9]+)[.]([0-9]+)[.]([0-9]+)$")
	releaseTagPattern = regexp.MustCompile("^v([0-9]+)[.]([0-9]+)[.]([0-9]+)(?:-[0-9A-Za-z.-]+)?$")
)

type gateError struct {
	message string
	cause   error
}

func newGateError(message string, cause error) gateError {
	return gateError{message: message, cause: cause}
}

func (err gateError) Error() string {
	if err.cause == nil {
		return err.message
	}

	return err.message + ": " + err.cause.Error()
}

func (err gateError) Unwrap() error {
	return err.cause
}

type version struct {
	major int
	minor int
	patch int
}

type incompatibleChange struct {
	packageName string
	details     string
}

type configuration struct {
	baseRef        string
	releaseVersion string
	policy         string
	modulePath     string
	packages       []string
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := parseConfiguration()
	if err != nil {
		return err
	}

	return execute(context.Background(), configuration)
}

func parseConfiguration() (configuration, error) {
	baseRef := flag.String("base", "", "Git ref to compare against, or previous-release")
	releaseVersion := flag.String("version", "", "release tag, used with -policy=release")
	policy := flag.String("policy", policyReport, "report or release")
	modulePath := flag.String("module", defaultModulePath, "module import path")
	packageList := flag.String(
		"packages",
		defaultPublicPackages,
		"comma-separated public package paths relative to the module root",
	)

	flag.Parse()

	if *baseRef == "" {
		return configuration{}, newGateError("-base is required", nil)
	}

	if *policy != policyReport && *policy != policyRelease {
		return configuration{}, newGateError("-policy must be report or release", nil)
	}

	if *policy == policyRelease && *releaseVersion == "" {
		return configuration{}, newGateError("-version is required with -policy=release", nil)
	}

	if strings.TrimSpace(*modulePath) == "" || strings.HasSuffix(*modulePath, "/") {
		return configuration{}, newGateError("-module must be a non-empty module import path", nil)
	}

	packages, err := parsePackages(*packageList)
	if err != nil {
		return configuration{}, newGateError("parse -packages", err)
	}

	return configuration{
		baseRef:        *baseRef,
		releaseVersion: *releaseVersion,
		policy:         *policy,
		modulePath:     *modulePath,
		packages:       packages,
	}, nil
}

func execute(ctx context.Context, configuration configuration) error {
	root, err := repositoryRoot(ctx)
	if err != nil {
		return newGateError("find repository root", err)
	}

	requestedBase := configuration.baseRef

	if isZeroRef(requestedBase) {
		requestedBase = previousRelease
	}

	base, found, err := resolveBase(ctx, root, requestedBase, configuration.releaseVersion)
	if err != nil {
		return newGateError("resolve baseline", err)
	}

	if !found {
		_, err = fmt.Fprintln(os.Stdout, "No previous stable release tag found; skipping API compatibility comparison.")
		if err != nil {
			return fmt.Errorf("write compatibility result: %w", err)
		}

		return nil
	}

	if configuration.policy == policyRelease {
		err = validateReleaseOrder(base, configuration.releaseVersion)
		if err != nil {
			return newGateError("validate release version", err)
		}
	}

	changes, err := collectChanges(ctx, root, base, configuration)
	if err != nil {
		return err
	}

	return finishReport(base, configuration, changes)
}

func collectChanges(
	ctx context.Context,
	root string,
	base string,
	configuration configuration,
) ([]incompatibleChange, error) {
	tempDir, err := os.MkdirTemp("", "go-api-compatibility-")
	if err != nil {
		return nil, newGateError("create temporary directory", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	baseDir := filepath.Join(tempDir, "base")

	err = git(ctx, root, "worktree", "add", "--detach", baseDir, base)
	if err != nil {
		return nil, newGateError("create baseline worktree", err)
	}

	defer func() {
		cleanupErr := git(ctx, root, "worktree", "remove", "--force", baseDir)
		if cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "remove baseline worktree: %v\n", cleanupErr)
		}
	}()

	toolDir := filepath.Join(tempDir, "bin")

	err = os.Mkdir(toolDir, toolDirectoryPermissions)
	if err != nil {
		return nil, newGateError("create tool directory", err)
	}

	err = installAPIDiff(ctx, root, toolDir)
	if err != nil {
		return nil, newGateError("install API comparison tool", err)
	}

	tool := filepath.Join(toolDir, "apidiff")

	if runtime.GOOS == "windows" {
		tool += ".exe"
	}

	return comparePackages(ctx, tool, root, baseDir, tempDir, configuration)
}

func comparePackages(
	ctx context.Context,
	tool string,
	root string,
	baseDir string,
	tempDir string,
	configuration configuration,
) ([]incompatibleChange, error) {
	changes := make([]incompatibleChange, 0)

	for _, packageName := range configuration.packages {
		packageChanges, err := comparePackage(
			ctx,
			tool,
			root,
			baseDir,
			tempDir,
			configuration.modulePath,
			packageName,
		)
		if err != nil {
			return nil, err
		}

		if packageChanges != "" {
			changes = append(changes, incompatibleChange{packageName: packageName, details: packageChanges})
		}
	}

	return changes, nil
}

func comparePackage(
	ctx context.Context,
	tool string,
	root string,
	baseDir string,
	tempDir string,
	modulePath string,
	packageName string,
) (string, error) {
	packagePath := fullPackagePath(modulePath, packageName)
	oldData := filepath.Join(tempDir, exportFilename(packageName, "base"))
	newData := filepath.Join(tempDir, exportFilename(packageName, "current"))

	err := writeExportData(ctx, tool, baseDir, packagePath, oldData)
	if err != nil {
		return "", newGateError("read baseline API for "+packagePath, err)
	}

	err = writeExportData(ctx, tool, root, packagePath, newData)
	if err != nil {
		return "", newGateError("read current API for "+packagePath, err)
	}

	packageChanges, err := compareAPIs(ctx, tool, oldData, newData)
	if err != nil {
		return "", newGateError("compare API for "+packagePath, err)
	}

	return packageChanges, nil
}

func exportFilename(packageName, variant string) string {
	return strings.ReplaceAll(packageName, "/", "-") + "-" + variant + ".export"
}

func finishReport(base string, configuration configuration, changes []incompatibleChange) error {
	releaseMessage, err := authorizeReleaseChanges(base, configuration, changes)
	if err != nil {
		return err
	}

	err = writeReportSummary(configuration.policy, base, changes, releaseMessage, false)
	if err != nil {
		return newGateError("write CI summary", err)
	}

	return printChanges(base, configuration, changes, releaseMessage)
}

func authorizeReleaseChanges(
	base string,
	configuration configuration,
	changes []incompatibleChange,
) (string, error) {
	if len(changes) == 0 || configuration.policy != policyRelease {
		return "", nil
	}

	allowed, reason, err := releaseAllowsBreak(base, configuration.releaseVersion)
	if err != nil {
		return "", newGateError("validate release version", err)
	}

	if allowed {
		return reason, nil
	}

	err = writeReportSummary(configuration.policy, base, changes, "", true)
	if err != nil {
		return "", newGateError("write CI summary", err)
	}

	message := fmt.Sprintf(
		"release %s has incompatible API changes from %s; the version increase does not permit these changes",
		configuration.releaseVersion,
		base,
	)

	return "", newGateError(message, nil)
}

func printChanges(
	base string,
	configuration configuration,
	changes []incompatibleChange,
	releaseMessage string,
) error {
	if len(changes) == 0 {
		_, err := fmt.Fprintf(os.Stdout, "Public Go API is compatible with %s.\n", base)
		if err != nil {
			return fmt.Errorf("write compatibility result: %w", err)
		}

		return nil
	}

	for _, change := range changes {
		_, err := fmt.Fprintf(os.Stdout, "Incompatible changes in %s:\n%s\n", change.packageName, change.details)
		if err != nil {
			return fmt.Errorf("write compatibility result: %w", err)
		}
	}

	if configuration.policy == policyReport {
		_, err := fmt.Fprintln(
			os.Stdout,
			"Report only: reviewers must explicitly acknowledge an intentional API break before approving.",
		)
		if err != nil {
			return fmt.Errorf("write compatibility result: %w", err)
		}

		return nil
	}

	_, err := fmt.Fprintf(
		os.Stdout,
		"Release %s permits these incompatible changes: %s.\n",
		configuration.releaseVersion,
		releaseMessage,
	)
	if err != nil {
		return fmt.Errorf("write compatibility result: %w", err)
	}

	return nil
}

func parsePackages(value string) ([]string, error) {
	packages := make([]string, 0)
	seen := make(map[string]struct{})

	for item := range strings.SplitSeq(value, ",") {
		name := strings.TrimSpace(item)
		if name == "" {
			return nil, newGateError("package paths must not be empty", nil)
		}

		if name != "." && (strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..")) {
			return nil, newGateError("package path must be relative to the module root: "+name, nil)
		}

		if _, exists := seen[name]; exists {
			return nil, newGateError("duplicate public package: "+name, nil)
		}

		seen[name] = struct{}{}

		packages = append(packages, name)
	}

	return packages, nil
}

func fullPackagePath(modulePath, packageName string) string {
	if packageName == "." {
		return modulePath
	}

	return modulePath + "/" + packageName
}

func repositoryRoot(ctx context.Context) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	output, err := runGit(ctx, workingDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find Git repository root: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func resolveBase(ctx context.Context, root, requested, releaseVersion string) (string, bool, error) {
	if requested != previousRelease {
		return requested, true, nil
	}

	tags, err := runGit(ctx, root, "tag", "--list", "v*", "--sort=-version:refname")
	if err != nil {
		return "", false, err
	}

	if releaseVersion == "" {
		return newestStableTag(string(tags))
	}

	target, err := parseVersion(releaseVersion, releaseTagPattern)
	if err != nil {
		return "", false, newGateError(
			"release version must be a vMAJOR.MINOR.PATCH tag, optionally with a prerelease suffix",
			err,
		)
	}

	return stableTagBefore(string(tags), releaseVersion, target)
}

func newestStableTag(tags string) (string, bool, error) {
	stableTags := strings.FieldsSeq(tags)
	for tag := range stableTags {
		_, err := parseVersion(tag, stableTagPattern)
		if err == nil {
			return tag, true, nil
		}
	}

	return "", false, nil
}

func stableTagBefore(tags, releaseVersion string, target version) (string, bool, error) {
	hasOtherStableTag := false

	stableTags := strings.FieldsSeq(tags)
	for tag := range stableTags {
		candidate, err := parseVersion(tag, stableTagPattern)
		if err != nil {
			continue
		}

		if tag != releaseVersion {
			hasOtherStableTag = true
		}

		if compareVersions(candidate, target) < 0 {
			return tag, true, nil
		}
	}

	if !hasOtherStableTag {
		return "", false, nil
	}

	return "", false, newGateError("no stable release tag is older than "+releaseVersion, nil)
}

func releaseAllowsBreak(baseTag, releaseTag string) (bool, string, error) {
	base, err := parseVersion(baseTag, stableTagPattern)
	if err != nil {
		return false, "", err
	}

	release, err := parseVersion(releaseTag, releaseTagPattern)
	if err != nil {
		return false, "", err
	}

	if release.major > base.major {
		return true, "the major version increased", nil
	}

	if base.major == 0 && release.major == 0 && release.minor > base.minor {
		return true, "the v0 minor version increased", nil
	}

	return false, "", nil
}

func validateReleaseOrder(baseTag, releaseTag string) error {
	base, err := parseVersion(baseTag, stableTagPattern)
	if err != nil {
		return err
	}

	release, err := parseVersion(releaseTag, releaseTagPattern)
	if err != nil {
		return err
	}

	if compareVersions(release, base) <= 0 {
		return newGateError(fmt.Sprintf("release tag %s must be newer than baseline %s", releaseTag, baseTag), nil)
	}

	return nil
}

func parseVersion(tag string, pattern *regexp.Regexp) (version, error) {
	match := pattern.FindStringSubmatch(tag)
	if len(match) != versionCaptureCount {
		return version{}, newGateError("invalid version tag "+tag, nil)
	}

	major, err := strconv.Atoi(match[1])
	if err != nil {
		return version{}, newGateError("parse major version in "+tag, err)
	}

	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return version{}, newGateError("parse minor version in "+tag, err)
	}

	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return version{}, newGateError("parse patch version in "+tag, err)
	}

	return version{major: major, minor: minor, patch: patch}, nil
}

func compareVersions(left, right version) int {
	if left.major != right.major {
		if left.major < right.major {
			return -1
		}

		return 1
	}

	if left.minor != right.minor {
		if left.minor < right.minor {
			return -1
		}

		return 1
	}

	if left.patch < right.patch {
		return -1
	}

	if left.patch > right.patch {
		return 1
	}

	return 0
}

func installAPIDiff(ctx context.Context, root, toolDir string) error {
	cmd := exec.CommandContext(ctx, "go", "install", apiDiffTool)
	cmd.Dir = root
	cmd.Env = withEnvironment(os.Environ(), "GOBIN", toolDir)
	cmd.Env = withEnvironment(cmd.Env, "GOTOOLCHAIN", "auto")
	cmd.Env = withEnvironment(cmd.Env, "GOWORK", "off")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("install API comparison tool: %w", err)
	}

	return nil
}

func writeExportData(ctx context.Context, tool, directory, packagePath, outputPath string) error {
	// #nosec G204 -- apidiff runs directly; paths and package names are arguments, not shell input.
	cmd := exec.CommandContext(ctx, tool, "-w", outputPath, packagePath)
	cmd.Dir = directory
	cmd.Env = withEnvironment(os.Environ(), "GOTOOLCHAIN", "auto")
	cmd.Env = withEnvironment(cmd.Env, "GOWORK", "off")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("run apidiff export: %w", err)
	}

	return nil
}

func compareAPIs(ctx context.Context, tool, oldData, newData string) (string, error) {
	// #nosec G204 -- apidiff runs directly; arguments are temporary file paths, not shell input.
	cmd := exec.CommandContext(ctx, tool, "-incompatible", oldData, newData)
	cmd.Env = withEnvironment(os.Environ(), "GOTOOLCHAIN", "auto")
	cmd.Env = withEnvironment(cmd.Env, "GOWORK", "off")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", newGateError(strings.TrimSpace(string(output)), err)
	}

	return strings.TrimSpace(string(output)), nil
}

func writeReportSummary(
	policy string,
	base string,
	changes []incompatibleChange,
	releaseMessage string,
	releaseFailed bool,
) error {
	var summary strings.Builder

	summary.WriteString("## Public Go API compatibility\n\n")
	summary.WriteString("Baseline: " + base + "\n\n")
	appendChangeSummary(&summary, policy, changes)
	appendReleaseSummary(&summary, policy, changes, releaseMessage, releaseFailed)

	text := summary.String()

	_, err := fmt.Fprint(os.Stdout, text)
	if err != nil {
		return fmt.Errorf("write API compatibility summary to standard output: %w", err)
	}

	path := os.Getenv("GITHUB_STEP_SUMMARY")

	if path == "" {
		return nil
	}

	file, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		summaryFilePermissions,
	) // #nosec G304 G703 -- GitHub provides the workflow-owned summary path.
	if err != nil {
		return fmt.Errorf("open GitHub step summary: %w", err)
	}

	_, writeErr := file.WriteString(text + "\n")

	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("write GitHub step summary: %w", writeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close GitHub step summary: %w", closeErr)
	}

	return nil
}

func appendChangeSummary(summary *strings.Builder, policy string, changes []incompatibleChange) {
	if len(changes) == 0 {
		summary.WriteString("No incompatible API changes were found.\n")

		return
	}

	summary.WriteString("Incompatible API changes were found.\n\n")

	if policy == policyReport {
		summary.WriteString(
			"This check reports changes without blocking merges. " +
				"Reviewers must explicitly acknowledge an intentional API break.\n\n",
		)
	} else {
		summary.WriteString("The release check evaluates these changes against the release version policy.\n\n")
	}

	for _, change := range changes {
		summary.WriteString("### " + change.packageName + "\n\n")
		summary.WriteString(change.details + "\n\n")
	}
}

func appendReleaseSummary(
	summary *strings.Builder,
	policy string,
	changes []incompatibleChange,
	releaseMessage string,
	releaseFailed bool,
) {
	if policy != policyRelease {
		return
	}

	switch {
	case releaseFailed:
		summary.WriteString(
			"Release compatibility check failed because the version does not permit the incompatible changes.\n",
		)
	case releaseMessage == "" && len(changes) == 0:
		summary.WriteString("Release compatibility check passed.\n")
	case releaseMessage != "":
		summary.WriteString("Release compatibility check passed: " + releaseMessage + ".\n")
	}
}

func isZeroRef(ref string) bool {
	return ref != "" && strings.Trim(ref, "0") == ""
}

func git(ctx context.Context, root string, args ...string) error {
	_, err := runGit(ctx, root, args...)

	return err
}

func runGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	gitArgs := append([]string{"-c", "safe.directory=" + filepath.ToSlash(root), "-C", root}, args...)
	// #nosec G204 -- fixed git executable and internal subcommands; no shell is invoked.
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run git command: %w", err)
	}

	return output, nil
}

func withEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(environment)+1)

	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}

	return append(filtered, prefix+value)
}
