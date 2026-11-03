// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// trim-crd-descriptions shortens long descriptions in generated CRDs to whole
// sentences so that CRDs embedding Kubernetes core types stay under the 256 KiB
// annotation limit of client-side `kubectl apply`.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

func main() {
	maxLen := flag.Int("max-len", 200, "descriptions longer than this are trimmed to the whole sentences that fit")
	flag.Parse()

	for _, path := range flag.Args() {
		if err := trimFile(path, *maxLen); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func trimFile(path string, maxLen int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var crd map[string]interface{}
	if err = yaml.Unmarshal(data, &crd); err != nil {
		return err
	}
	trimDescriptions(crd, maxLen)

	out, err := yaml.Marshal(crd)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte("---\n")) {
		out = append([]byte("---\n"), out...)
	}

	return os.WriteFile(path, out, 0644)
}

func trimDescriptions(node interface{}, maxLen int) {
	switch n := node.(type) {
	case map[string]interface{}:
		for key, value := range n {
			if description, ok := value.(string); ok && key == "description" {
				n[key] = trimDescription(description, maxLen)
				continue
			}
			trimDescriptions(value, maxLen)
		}
	case []interface{}:
		for _, item := range n {
			trimDescriptions(item, maxLen)
		}
	}
}

// trimDescription keeps the first paragraph's leading sentences that fit in
// maxLen, and always at least the first sentence.
func trimDescription(description string, maxLen int) string {
	if len(description) <= maxLen {
		return description
	}

	paragraph, _, _ := strings.Cut(description, "\n\n")
	end := 0
	for _, sentenceEnd := range sentenceEnds(paragraph) {
		if end > 0 && sentenceEnd > maxLen {
			break
		}
		end = sentenceEnd
	}
	if end == 0 {
		return paragraph
	}

	return paragraph[:end]
}

// sentenceEnds returns the offsets just past each sentence-ending period.
func sentenceEnds(text string) []int {
	var ends []int
	for i := 0; i < len(text); i++ {
		if text[i] != '.' {
			continue
		}
		if i+1 < len(text) && text[i+1] != ' ' && text[i+1] != '\n' {
			continue
		}
		if isAbbreviation(text[:i]) {
			continue
		}
		ends = append(ends, i+1)
	}

	return ends
}

func isAbbreviation(textBeforePeriod string) bool {
	for _, abbreviation := range []string{"e.g", "i.e", "etc", "vs"} {
		if strings.HasSuffix(textBeforePeriod, abbreviation) {
			return true
		}
	}

	return false
}
