// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// trim-crd-descriptions shortens the descriptions that generated CRDs copy from
// Kubernetes core types to whole sentences, so that CRDs embedding those types
// stay under the 256 KiB annotation limit of client-side `kubectl apply`.
// Descriptions of the operator's own fields and types are kept in full.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	mmv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mmv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

const operatorAPIsPkgPrefix = "github.com/mattermost/mattermost-operator/apis/"

func main() {
	maxLen := flag.Int("max-len", 200, "descriptions longer than this are trimmed to the whole sentences that fit")
	flag.Parse()

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{mmv1alpha1.AddToScheme, mmv1beta1.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	for _, path := range flag.Args() {
		if err := trimFile(path, scheme, *maxLen); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func trimFile(path string, scheme *runtime.Scheme, maxLen int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var crd map[string]interface{}
	if err = yaml.Unmarshal(data, &crd); err != nil {
		return err
	}
	if err = trimCRD(crd, scheme, maxLen); err != nil {
		return err
	}

	out, err := yaml.Marshal(crd)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte("---\n")) {
		out = append([]byte("---\n"), out...)
	}

	return os.WriteFile(path, out, 0644)
}

func trimCRD(crd map[string]interface{}, scheme *runtime.Scheme, maxLen int) error {
	spec, _ := crd["spec"].(map[string]interface{})
	group, _ := spec["group"].(string)
	names, _ := spec["names"].(map[string]interface{})
	kind, _ := names["kind"].(string)
	versions, _ := spec["versions"].([]interface{})

	for _, item := range versions {
		version, _ := item.(map[string]interface{})
		versionName, _ := version["name"].(string)
		gvk := schema.GroupVersionKind{Group: group, Version: versionName, Kind: kind}
		goType, ok := scheme.AllKnownTypes()[gvk]
		if !ok {
			return fmt.Errorf("no Go type registered for %s", gvk)
		}

		versionSchema, _ := version["schema"].(map[string]interface{})
		openAPISchema, _ := versionSchema["openAPIV3Schema"].(map[string]interface{})
		if err := trimSchema(openAPISchema, goType, true, maxLen); err != nil {
			return fmt.Errorf("%s: %w", gvk, err)
		}
	}

	return nil
}

// trimSchema walks node alongside the Go type it was generated from, trimming
// descriptions unless they document the operator's own fields and types.
func trimSchema(node map[string]interface{}, goType reflect.Type, operatorOwned bool, maxLen int) error {
	if description, ok := node["description"].(string); ok && !operatorOwned {
		node["description"] = trimDescription(description, maxLen)
	}
	goType = indirect(goType)

	if properties, ok := node["properties"].(map[string]interface{}); ok {
		fields := jsonFields(goType)
		for name, property := range properties {
			field, ok := fields[name]
			if !ok {
				return fmt.Errorf("no Go field for property %q of %s", name, goType)
			}
			propertySchema, _ := property.(map[string]interface{})
			if err := trimSchema(propertySchema, field.goType, isOperatorType(field.declaredIn), maxLen); err != nil {
				return err
			}
		}
	}

	for _, key := range []string{"items", "additionalProperties"} {
		subschema, ok := node[key].(map[string]interface{})
		if !ok {
			continue
		}
		if !slices.Contains([]reflect.Kind{reflect.Slice, reflect.Array, reflect.Map}, goType.Kind()) {
			return fmt.Errorf("schema has %s but Go type %s has no elements", key, goType)
		}
		if err := trimSchema(subschema, goType.Elem(), isOperatorType(goType.Elem()), maxLen); err != nil {
			return err
		}
	}

	return nil
}

type jsonField struct {
	goType     reflect.Type
	declaredIn reflect.Type
}

// jsonFields maps the JSON names of a struct's fields, including those of
// inlined structs, to their type and the struct declaring them.
func jsonFields(goType reflect.Type) map[string]jsonField {
	fields := map[string]jsonField{}
	if goType.Kind() != reflect.Struct {
		return fields
	}

	for i := 0; i < goType.NumField(); i++ {
		field := goType.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" || (!field.IsExported() && !field.Anonymous) {
			continue
		}
		if name == "" && (field.Anonymous || slices.Contains(strings.Split(options, ","), "inline")) {
			maps.Copy(fields, jsonFields(indirect(field.Type)))
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = jsonField{goType: field.Type, declaredIn: goType}
	}

	return fields
}

func isOperatorType(goType reflect.Type) bool {
	return strings.HasPrefix(indirect(goType).PkgPath(), operatorAPIsPkgPrefix)
}

func indirect(goType reflect.Type) reflect.Type {
	for goType.Kind() == reflect.Pointer {
		goType = goType.Elem()
	}

	return goType
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
