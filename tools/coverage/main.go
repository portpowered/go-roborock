// Command coverage reports statement coverage for non-generated library code.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultMinimumCoverage = 80
	coverageRecordFields   = 3
	coverageFileMode       = 0o600
	moduleDirectiveFields  = 2
	generatedHeaderLimit   = 2048
)

var (
	errConflictingCoverageModes = errors.New("coverage profile has conflicting modes")
	errInvalidCoverageRecord    = errors.New("invalid coverage record")
	errInvalidCoverageLocation  = errors.New("invalid coverage location")
	errInvalidStatementCount    = errors.New("invalid statement count")
	errInvalidExecutionCount    = errors.New("invalid execution count")
	errConflictingStatementNums = errors.New("coverage block has conflicting statement counts")
	errMissingCoverageMode      = errors.New("coverage profile has no mode header")
	errNoProductionStatements   = errors.New("coverage profile has no non-generated pkg statements")
	errCoverageBelowMinimum     = errors.New("coverage is below the minimum")
	errMissingModuleDirective   = errors.New("module directive not found")
)

type totals struct {
	covered int64
	all     int64
}

type coverageBlock struct {
	source     string
	statements int64
	covered    bool
}

type parsedRecord struct {
	location   string
	source     string
	statements int64
	count      int64
	include    bool
}

type fileReadResult struct {
	data []byte
	err  error
}

type generatedFileResult struct {
	generated bool
	err       error
}

type coverageNumberResult struct {
	value int64
	err   error
}

func main() {
	profile := flag.String("profile", "coverage.out", "Go coverage profile")
	minimum := flag.Float64("min", defaultMinimumCoverage, "minimum combined statement coverage percent")
	percentOnly := flag.Bool("percent-only", false, "print only the combined percentage")
	filteredProfile := flag.String("filtered-profile", "", "write a profile without generated files")

	flag.Parse()

	err := report(".", *profile, *minimum, *percentOnly, *filteredProfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func report(root, profile string, minimum float64, percentOnly bool, filteredProfile string) error {
	module, err := modulePath(resolvePath(root, "go.mod"))
	if err != nil {
		return err
	}

	blocks, mode, err := readCoverageProfile(root, profile, module)
	if err != nil {
		return err
	}

	byPackage, filtered := aggregateBlocks(blocks, mode)
	if filteredProfile != "" {
		writeErr := os.WriteFile(resolvePath(root, filteredProfile), []byte(filtered), coverageFileMode)
		if writeErr != nil {
			return fmt.Errorf("write filtered coverage profile: %w", writeErr)
		}
	}

	return printCoverageReport(byPackage, minimum, percentOnly)
}

func readCoverageProfile(root, profile, module string) (map[string]coverageBlock, string, error) {
	file, err := os.Open(resolvePath(root, profile)) // #nosec G304 -- profile is an explicit local CLI input.
	if err != nil {
		return nil, "", fmt.Errorf("open coverage profile: %w", err)
	}

	blocks := make(map[string]coverageBlock)
	generated := make(map[string]bool)
	mode := ""
	scanner := bufio.NewScanner(file)

	var parseErr error

	for scanner.Scan() {
		mode, err = consumeCoverageLine(scanner.Text(), root, module, generated, blocks, mode)
		if err != nil {
			parseErr = err

			break
		}
	}

	if parseErr == nil {
		scanErr := scanner.Err()
		if scanErr != nil {
			parseErr = fmt.Errorf("read coverage profile: %w", scanErr)
		}
	}

	closeErr := file.Close()

	if parseErr != nil {
		return nil, "", errors.Join(parseErr, closeError(closeErr))
	}

	if closeErr != nil {
		return nil, "", closeError(closeErr)
	}

	if mode == "" {
		return nil, "", errMissingCoverageMode
	}

	return blocks, mode, nil
}

func closeError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("close coverage profile: %w", err)
}

func consumeCoverageLine(
	line string,
	root string,
	module string,
	generated map[string]bool,
	blocks map[string]coverageBlock,
	mode string,
) (string, error) {
	if strings.HasPrefix(line, "mode:") {
		if mode != "" && mode != line {
			return mode, fmt.Errorf("%w: %q and %q", errConflictingCoverageModes, mode, line)
		}

		return line, nil
	}

	record, err := parseCoverageRecord(root, line, module, generated)
	if err != nil {
		return mode, err
	}

	if record.include {
		err = mergeCoverageBlock(blocks, record)
		if err != nil {
			return mode, err
		}
	}

	return mode, nil
}

func parseCoverageRecord(root, line, module string, generated map[string]bool) (parsedRecord, error) {
	fields := strings.Fields(line)
	if len(fields) != coverageRecordFields {
		return parsedRecord{}, fmt.Errorf("%w: %q", errInvalidCoverageRecord, line)
	}

	location := fields[0]

	source, include, err := coverageSource(location, module)
	if err != nil {
		return parsedRecord{}, err
	}

	if !include {
		return parsedRecord{location: "", source: "", statements: 0, count: 0, include: false}, nil
	}

	isGenerated, ok := generated[source]
	if !ok {
		inspection := inspectGeneratedFile(resolvePath(root, source))
		if inspection.err != nil {
			return parsedRecord{}, inspection.err
		}

		isGenerated = inspection.generated

		generated[source] = isGenerated
	}

	if isGenerated {
		return parsedRecord{location: "", source: "", statements: 0, count: 0, include: false}, nil
	}

	statements := coverageNumber(fields[1], errInvalidStatementCount, line)
	if statements.err != nil {
		return parsedRecord{}, statements.err
	}

	count := coverageNumber(fields[2], errInvalidExecutionCount, line)
	if count.err != nil {
		return parsedRecord{}, count.err
	}

	return parsedRecord{
		location:   location,
		source:     source,
		statements: statements.value,
		count:      count.value,
		include:    true,
	}, nil
}

func coverageSource(location, module string) (string, bool, error) {
	colon := strings.LastIndexByte(location, ':')
	if colon < 0 {
		return "", false, fmt.Errorf("%w: %q", errInvalidCoverageLocation, location)
	}

	source := strings.TrimPrefix(strings.ReplaceAll(location[:colon], "\\", "/"), module+"/")

	if !strings.HasPrefix(source, "pkg/") || strings.HasPrefix(source, "pkg/testing/") {
		return source, false, nil
	}

	return source, true, nil
}

func coverageNumber(value string, invalid error, line string) coverageNumberResult {
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count < 0 {
		return coverageNumberResult{value: 0, err: fmt.Errorf("%w: %q", invalid, line)}
	}

	return coverageNumberResult{value: count, err: nil}
}

func mergeCoverageBlock(blocks map[string]coverageBlock, record parsedRecord) error {
	previous, exists := blocks[record.location]
	if !exists {
		blocks[record.location] = coverageBlock{
			source:     record.source,
			statements: record.statements,
			covered:    record.count > 0,
		}

		return nil
	}

	if previous.statements != record.statements {
		return fmt.Errorf("%w: %q", errConflictingStatementNums, record.location)
	}

	previous.covered = previous.covered || record.count > 0
	blocks[record.location] = previous

	return nil
}

func aggregateBlocks(blocks map[string]coverageBlock, mode string) (map[string]totals, string) {
	keys := make([]string, 0, len(blocks))
	for location := range blocks {
		keys = append(keys, location)
	}

	sort.Strings(keys)

	byPackage := make(map[string]totals)

	var filtered strings.Builder

	filtered.WriteString(mode + "\n")

	for _, location := range keys {
		block := blocks[location]
		if block.statements < 0 {
			continue
		}

		count := 0

		if block.covered {
			count = 1
		}

		fmt.Fprintf(&filtered, "%s %d %d\n", location, block.statements, count)
		packageName := path.Dir(block.source)
		entry := byPackage[packageName]

		entry.all += block.statements
		if block.covered {
			entry.covered += block.statements
		}

		byPackage[packageName] = entry
	}

	return byPackage, filtered.String()
}

func printCoverageReport(byPackage map[string]totals, minimum float64, percentOnly bool) error {
	packages := make([]string, 0, len(byPackage))
	for name := range byPackage {
		packages = append(packages, name)
	}

	sort.Strings(packages)

	combined, err := packageTotals(packages, byPackage, percentOnly)
	if err != nil {
		return err
	}

	if combined.all == 0 {
		return errNoProductionStatements
	}

	measured := percent(combined)
	if percentOnly {
		writeErr := writeCoverageLine("%.1f\n", measured)
		if writeErr != nil {
			return writeErr
		}
	} else {
		writeErr := writeCoverageLine(
			"combined non-generated pkg coverage: %.1f%% (%d/%d statements)\n",
			measured,
			combined.covered,
			combined.all,
		)
		if writeErr != nil {
			return writeErr
		}
	}

	if measured+1e-9 < minimum {
		return fmt.Errorf("%w: %.1f%% is below %.1f%%", errCoverageBelowMinimum, measured, minimum)
	}

	return nil
}

func packageTotals(packages []string, byPackage map[string]totals, percentOnly bool) (totals, error) {
	var combined totals

	for _, name := range packages {
		entry := byPackage[name]
		combined.all += entry.all

		combined.covered += entry.covered

		if entry.all > 0 && !percentOnly {
			writeErr := writeCoverageLine(
				"%s: %.1f%% (%d/%d statements)\n",
				name,
				percent(entry),
				entry.covered,
				entry.all,
			)
			if writeErr != nil {
				return totals{}, writeErr
			}
		}
	}

	return combined, nil
}

func writeCoverageLine(format string, values ...any) error {
	_, err := fmt.Fprintf(os.Stdout, format, values...)
	if err != nil {
		return fmt.Errorf("write coverage report: %w", err)
	}

	return nil
}

func modulePath(filename string) (string, error) {
	file := readLocalFile(filename)
	if file.err != nil {
		return "", fmt.Errorf("read module file: %w", file.err)
	}

	for line := range strings.SplitSeq(string(file.data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == moduleDirectiveFields && fields[0] == "module" {
			return fields[1], nil
		}
	}

	return "", fmt.Errorf("%w: %s", errMissingModuleDirective, filename)
}

func resolvePath(root, filename string) string {
	if filepath.IsAbs(filename) {
		return filename
	}

	return filepath.Join(root, filename)
}

func generatedFile(filename string) (bool, error) {
	result := inspectGeneratedFile(filename)

	return result.generated, result.err
}

func inspectGeneratedFile(filename string) generatedFileResult {
	if strings.HasSuffix(filename, ".gen.go") {
		return generatedFileResult{generated: true, err: nil}
	}

	file := readLocalFile(filename)
	if file.err != nil {
		return generatedFileResult{
			generated: false,
			err:       fmt.Errorf("read source file %s: %w", filename, file.err),
		}
	}

	if len(file.data) > generatedHeaderLimit {
		file.data = file.data[:generatedHeaderLimit]
	}

	return generatedFileResult{
		generated: strings.Contains(string(file.data), "Code generated") &&
			strings.Contains(string(file.data), "DO NOT EDIT"),
		err: nil,
	}
}

func readLocalFile(filename string) fileReadResult {
	data, err := os.ReadFile(filename) // #nosec G304 -- explicit local module or coverage input.

	return fileReadResult{data: data, err: err}
}

func percent(value totals) float64 {
	return 100 * float64(value.covered) / float64(value.all)
}
