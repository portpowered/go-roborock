package main

import (
	"testing"
	"testing/fstest"
	"time"
)

func TestCheckSite(t *testing.T) {
	t.Parallel()

	tree := fstest.MapFS{
		"index.html": mapFile(`<a href="/go-roborock/guide#hello">Guide</a><img src="/go-roborock/icon.svg">`),
		"guide.html": mapFile(`<h1 id="hello">Hello</h1><a href="./index.html">Home</a>` +
			`<a href="https://example.com">External</a><style href="react-style">body{}</style>` +
			`<script>{"href":"not-a-link"}</script>`),
		"icon.svg": mapFile(`<svg/>`),
	}

	err := checkSite(tree, "/go-roborock")
	if err != nil {
		t.Fatal(err)
	}

	links := []string{"missing", "/elsewhere", "/go-roborock/guide#absent", "../../private", "/go-roborock/../private"}

	for _, link := range links {
		err := checkLink(tree, "/go-roborock", "index.html", link)
		if err == nil {
			t.Errorf("accepted broken destination %s", link)
		}
	}

	err = checkSite(fstest.MapFS{}, "/go-roborock")
	if err == nil {
		t.Error("accepted an empty site")
	}
}

func mapFile(data string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(data), Mode: 0o600, ModTime: time.Time{}, Sys: nil}
}
