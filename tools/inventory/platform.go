package main

import (
	"slices"
	"sort"
	"strings"
)

const inventoryLinuxTarget = "linux"

type declarationKey struct {
	symbol     string
	kind       string
	definition site
	schema     schemaOwner
	generator  string
	value      string
}

func inventoryEnvironment(current []string, workspace, target string) []string {
	result := make([]string, 0, len(current))

	for _, entry := range current {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case "GOWORK", "GOOS", "GOARCH", "CGO_ENABLED":
		default:
			result = append(result, entry)
		}
	}

	return append(result, "GOWORK="+workspace, "GOOS="+target, "GOARCH=amd64", "CGO_ENABLED=0")
}

func emptyReport() report {
	return report{Declarations: []declaration{}, Behavior: []site{}, Aliases: []site{},
		Codecs: []codec{}, Boundaries: []boundary{}}
}

func mergeReports(platforms []report) report {
	result := emptyReport()

	declarations := make(map[declarationKey]int)

	for _, platform := range platforms {
		mergeDeclarations(&result, declarations, platform.Declarations)
		result.Behavior = append(result.Behavior, platform.Behavior...)
		result.Aliases = append(result.Aliases, platform.Aliases...)
		result.Codecs = append(result.Codecs, platform.Codecs...)
		result.Boundaries = append(result.Boundaries, platform.Boundaries...)
	}

	normalizeReport(&result)
	result.Behavior = slices.Compact(result.Behavior)
	result.Aliases = slices.Compact(result.Aliases)
	result.Codecs = uniqueCodecs(result.Codecs)

	result.Boundaries = uniqueBoundaries(result.Boundaries)

	for index := range result.Declarations {
		result.Declarations[index].Uses = slices.Compact(result.Declarations[index].Uses)
	}

	return result
}

func uniqueCodecs(codecs []codec) []codec {
	sort.Slice(codecs, func(left, right int) bool {
		if codecs[left].Definition != codecs[right].Definition {
			return siteLess(codecs[left].Definition, codecs[right].Definition)
		}

		return slices.Compare([]string{codecs[left].Schema.File, codecs[left].Schema.Pointer},
			[]string{codecs[right].Schema.File, codecs[right].Schema.Pointer}) < 0
	})

	return slices.Compact(codecs)
}

func mergeDeclarations(result *report, entries map[declarationKey]int, declarations []declaration) {
	for _, item := range declarations {
		key := declarationKey{symbol: item.Symbol, kind: item.Kind, definition: item.Definition,
			schema: item.Schema, generator: item.Generator, value: item.Value}
		if row, exists := entries[key]; exists {
			result.Declarations[row].Uses = append(result.Declarations[row].Uses, item.Uses...)
		} else {
			item.Uses = append([]site{}, item.Uses...)
			entries[key] = len(result.Declarations)
			result.Declarations = append(result.Declarations, item)
		}
	}
}

func uniqueBoundaries(boundaries []boundary) []boundary {
	sort.Slice(boundaries, func(left, right int) bool {
		if boundaries[left].Site != boundaries[right].Site {
			return siteLess(boundaries[left].Site, boundaries[right].Site)
		}

		return slices.Compare(boundaries[left].Arguments, boundaries[right].Arguments) < 0
	})

	return slices.CompactFunc(boundaries, func(left, right boundary) bool {
		return left.Site == right.Site && slices.Equal(left.Arguments, right.Arguments)
	})
}
