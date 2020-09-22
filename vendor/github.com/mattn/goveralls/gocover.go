package main

// Much of the core of this is copied from go's cover tool itself.

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The rest is written by Dustin Sallings

import (
	"bytes"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"
)

func findFile(file string) (string, error) {
	dir, file := filepath.Split(file)
	pkg, err := build.Import(dir, ".", build.FindOnly)
	if err != nil {
		return "", fmt.Errorf("can't find %q: %v", file, err)
	}
	return filepath.Join(pkg.Dir, file), nil
}

// mergeProfs merges profiles for same target packages.
// It assumes each profiles have same sorted FileName and Blocks.
func mergeProfs(pfss [][]*cover.Profile) []*cover.Profile {
	if len(pfss) == 0 {
		return nil
	}
	for len(pfss) > 1 {
		i := 0
		for ; 2*i+1 < len(pfss); i++ {
			pfss[i] = mergeTwoProfs(pfss[2*i], pfss[2*i+1])
		}
		if 2*i < len(pfss) {
			pfss[i] = pfss[2*i]
			i++
		}
		pfss = pfss[:i]
	}
	return pfss[0]
}

func mergeTwoProfs(left, right []*cover.Profile) []*cover.Profile {
	ret := make([]*cover.Profile, 0, len(left)+len(right))
	for len(left) > 0 && len(right) > 0 {
		if left[0].FileName == right[0].FileName {
			profile := &cover.Profile{
				FileName: left[0].FileName,
				Mode:     left[0].Mode,
				Blocks:   mergeTwoProfBlock(left[0].Blocks, right[0].Blocks),
			}
			ret = append(ret, profile)
			left = left[1:]
			right = right[1:]
		} else if left[0].FileName < right[0].FileName {
			ret = append(ret, left[0])
			left = left[1:]
		} else {
			ret = append(ret, right[0])
			right = right[1:]
		}
	}
	ret = append(ret, left...)
	ret = append(ret, right...)
	return ret
}

func mergeTwoProfBlock(left, right []cover.ProfileBlock) []cover.ProfileBlock {
	ret := make([]cover.ProfileBlock, 0, len(left)+len(right))
	for len(left) > 0 && len(right) > 0 {
		a, b := left[0], right[0]
		if a.StartLine == b.StartLine && a.StartCol == b.StartCol && a.EndLine == b.EndLine && a.EndCol == b.EndCol {
			ret = append(ret, cover.ProfileBlock{
				StartLine: a.StartLine,
				StartCol:  a.StartCol,
				EndLine:   a.EndLine,
				EndCol:    a.EndCol,
				NumStmt:   a.NumStmt,
				Count:     a.Count + b.Count,
			})
			left = left[1:]
			right = right[1:]
		} else if a.StartLine < b.StartLine || (a.StartLine == b.StartLine && a.StartCol < b.StartCol) {
			ret = append(ret, a)
			left = left[1:]
		} else {
			ret = append(ret, b)
			right = right[1:]
		}
	}
	ret = append(ret, left...)
	ret = append(ret, right...)
	return ret
}

// toSF converts profiles to sourcefiles for coveralls.
func toSF(profs []*cover.Profile) ([]*SourceFile, error) {
	var rv []*SourceFile
	for _, prof := range profs {
		path, err := findFile(prof.FileName)
		if err != nil {
			log.Fatalf("Can't find %v", err)
		}
		fb, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatalf("Error reading %v: %v", path, err)
		}
		sf := &SourceFile{
			Name:     getCoverallsSourceFileName(path),
			Source:   string(fb),
			Coverage: make([]interface{}, 1+bytes.Count(fb, []byte{'\n'})),
		}

		for _, block := range prof.Blocks {
			for i := block.StartLine; i <= block.EndLine; i++ {
				count, _ := sf.Coverage[i-1].(int)
				sf.Coverage[i-1] = count + block.Count
			}
		}

		rv = append(rv, sf)
	}

	return rv, nil
}

func parseCover(fn string) ([]*SourceFile, error) {
	var pfss [][]*cover.Profile
	for _, p := range strings.Split(fn, ",") {
		profs, err := cover.ParseProfiles(p)
		if err != nil {
			return nil, fmt.Errorf("Error parsing coverage: %v", err)
		}
		pfss = append(pfss, profs)
	}

	sourceFiles, err := toSF(mergeProfs(pfss))
	if err != nil {
		return nil, err
	}

	return sourceFiles, nil
}
