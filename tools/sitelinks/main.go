// Command sitelinks verifies local destinations in every rendered HTML page.
package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

var (
	attributes    = regexp.MustCompile(`(?i)\b(href|src|id)\s*=\s*["']([^"']*)["']`)
	tags          = regexp.MustCompile(`(?is)<([a-z][a-z0-9]*)\b[^>]*>`)
	scriptContent = regexp.MustCompile(`(?is)(<script\b[^>]*>).*?</script>`)
	errMissing    = errors.New("missing local destination")
	errNoPages    = errors.New("website contains no rendered HTML pages")
)

func main() {
	root := flag.String("root", "site", "rendered website directory")
	base := flag.String("base", "/go-roborock", "project URL prefix")

	flag.Parse()

	err := checkSite(os.DirFS(*root), *base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkSite(tree fs.FS, base string) error {
	pages := 0

	err := fs.WalkDir(tree, ".", func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk website: %w", walkErr)
		}

		if entry.IsDir() || !strings.HasSuffix(file, ".html") {
			return nil
		}

		pages++

		return checkPage(tree, base, file)
	})
	if err != nil {
		return fmt.Errorf("check website: %w", err)
	}

	if pages == 0 {
		return errNoPages
	}

	return nil
}

func checkPage(tree fs.FS, base, file string) error {
	data, err := fs.ReadFile(tree, file)
	if err != nil {
		return fmt.Errorf("read page: %w", err)
	}

	for _, tag := range pageTags(data) {
		for _, attr := range attributes.FindAllStringSubmatch(tag[0], -1) {
			if !isLinkAttribute(tag[1], attr[1]) {
				continue
			}

			err := checkLink(tree, base, file, html.UnescapeString(attr[2]))
			if err != nil {
				return fmt.Errorf("%s: %s: %w", file, attr[2], err)
			}
		}
	}

	return nil
}

func pageTags(data []byte) [][]string {
	return tags.FindAllStringSubmatch(scriptContent.ReplaceAllString(string(data), "$1"), -1)
}

func isLinkAttribute(tag, attribute string) bool {
	if strings.EqualFold(attribute, "src") {
		return true
	}

	return strings.EqualFold(attribute, "href") && (strings.EqualFold(tag, "a") || strings.EqualFold(tag, "link"))
}

func checkLink(tree fs.FS, base, source, link string) error {
	parsed, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	if parsed.IsAbs() || parsed.Host != "" {
		return nil
	}

	target, err := localTarget(base, source, parsed.Path)
	if err != nil {
		return err
	}

	target, err = existingTarget(tree, target)
	if err != nil {
		return err
	}

	if parsed.Fragment == "" || !strings.HasSuffix(target, ".html") {
		return nil
	}

	return checkFragment(tree, target, parsed.Fragment)
}

func localTarget(base, source, linkPath string) (string, error) {
	if linkPath == "" {
		return source, nil
	}

	if !strings.HasPrefix(linkPath, "/") {
		target := path.Join(path.Dir(source), linkPath)
		if !fs.ValidPath(target) {
			return "", fmt.Errorf("outside website: %w", errMissing)
		}

		return target, nil
	}

	if base != "" && linkPath != base && !strings.HasPrefix(linkPath, base+"/") {
		return "", fmt.Errorf("outside project base: %w", errMissing)
	}

	target := strings.TrimPrefix(strings.TrimPrefix(linkPath, base), "/")
	if target == "" {
		return ".", nil
	}

	target = path.Clean(target)
	if !fs.ValidPath(target) {
		return "", fmt.Errorf("outside website: %w", errMissing)
	}

	return target, nil
}

func existingTarget(tree fs.FS, target string) (string, error) {
	info, err := fs.Stat(tree, target)
	if err == nil && info.IsDir() {
		target = path.Join(target, "index.html")
	}

	_, err = fs.Stat(tree, target)
	if err == nil {
		return target, nil
	}

	_, err = fs.Stat(tree, target+".html")
	if err != nil {
		return "", fmt.Errorf("%s: %w", target, errMissing)
	}

	return target + ".html", nil
}

func checkFragment(tree fs.FS, target, fragment string) error {
	data, err := fs.ReadFile(tree, target)
	if err != nil {
		return fmt.Errorf("read destination: %w", err)
	}

	for _, tag := range pageTags(data) {
		for _, attr := range attributes.FindAllStringSubmatch(tag[0], -1) {
			if strings.EqualFold(attr[1], "id") && html.UnescapeString(attr[2]) == fragment {
				return nil
			}
		}
	}

	return fmt.Errorf("fragment %s in %s: %w", fragment, target, errMissing)
}
