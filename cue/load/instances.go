// Copyright 2018 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package load

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/internal/filetypes"
	"cuelang.org/go/internal/mod/modimports"
	"cuelang.org/go/internal/mod/modpkgload"
	"cuelang.org/go/internal/mod/modrequirements"
	"cuelang.org/go/mod/module"
)

// Instances returns the instances named by the command line arguments 'args'.
// If errors occur trying to load an instance it is returned with Incomplete
// set. Errors directly related to loading the instance are recorded in this
// instance, but errors that occur loading dependencies are recorded in these
// dependencies.
func Instances(args []string, c *Config) []*build.Instance {
	ctx := context.TODO()
	if c == nil {
		c = &Config{}
	}
	newC, err := c.complete()
	if err != nil {
		return []*build.Instance{c.newErrInstance(err)}
	}
	c = newC
	if len(args) == 0 {
		args = []string{"."}
	}
	// TODO: This requires packages to be placed before files. At some point this
	// could be relaxed.
	i := 0
	for ; i < len(args) && filetypes.IsPackage(args[i]); i++ {
	}
	pkgArgs := args[:i]
	otherArgs := args[i:]
	otherFiles, err := filetypes.ParseArgs(otherArgs)
	if err != nil {
		return []*build.Instance{c.newErrInstance(err)}
	}
	for _, f := range otherFiles {
		if err := setFileSource(c, f); err != nil {
			return []*build.Instance{c.newErrInstance(err)}
		}
	}
	if c.Package != "" && c.Package != "_" && c.Package != "*" {
		// The caller has specified an explicit package to load.
		// This is essentially the same as passing an explicit package
		// qualifier to all package arguments that don't already have
		// one. We add that qualifier here so that there's a distinction
		// between package paths specified as arguments, which
		// have the qualifier added, and package paths that are dependencies
		// of those, which don't.
		pkgArgs1 := make([]string, 0, len(pkgArgs))
		for _, p := range pkgArgs {
			if ip := module.ParseImportPath(p); !ip.ExplicitQualifier {
				ip.Qualifier = c.Package
				p = ip.String()
			}
			pkgArgs1 = append(pkgArgs1, p)
		}
		pkgArgs = pkgArgs1
	}

	tg := newTagger(c)
	// Pass all arguments that look like packages to loadPackages
	// so that they'll be available when looking up the packages
	// that are specified on the command line.
	expandedPaths, err := expandPackageArgs(c, pkgArgs, c.Package, tg)
	if err != nil {
		return []*build.Instance{c.newErrInstance(err)}
	}

	var pkgs *modpkgload.Packages
	if !c.SkipImports {
		pkgs, err = loadPackages(ctx, c, expandedPaths, otherFiles, tg)
		if err != nil {
			return []*build.Instance{c.newErrInstance(err)}
		}
	}
	l := newLoader(c, tg, pkgs)

	if c.Context == nil {
		opts := []build.Option{
			build.ParseFile(c.ParseFile),
		}
		if f := l.loadFunc(); l != nil {
			opts = append(opts, build.Loader(f))
		}
		c.Context = build.NewContext(opts...)
	}

	a := []*build.Instance{}
	if len(pkgArgs) > 0 {
		for _, m := range l.importPaths(pkgArgs) {
			if m.Err != nil {
				inst := c.newErrInstance(m.Err)
				a = append(a, inst)
				continue
			}
			a = append(a, m.Pkgs...)
		}
	}

	if len(otherFiles) > 0 {
		a = append(a, l.cueFilesPackage(otherFiles))
	}

	for _, p := range a {
		tags, err := findTags(p)
		if err != nil {
			p.ReportError(err)
		}
		tg.tags = append(tg.tags, tags...)
	}

	// TODO(api): have API call that returns an error which is the aggregate
	// of all build errors. Certain errors, like these, hold across builds.
	if err := tg.injectTags(c.Tags); err != nil {
		for _, p := range a {
			p.ReportError(err)
		}
		return a
	}

	if tg.replacements == nil {
		return a
	}

	for _, p := range a {
		for _, f := range p.Files {
			ast.Walk(f, nil, func(n ast.Node) {
				if ident, ok := n.(*ast.Ident); ok {
					if v, ok := tg.replacements[ident.Node]; ok {
						ident.Node = v
					}
				}
			})
		}
	}

	return a
}

// loadPackages returns packages loaded from the given package list and also
// including imports from the given build files.
func loadPackages(
	ctx context.Context,
	cfg *Config,
	pkgs []resolvedPackageArg,
	otherFiles []*build.File,
	tg *tagger,
) (*modpkgload.Packages, error) {
	if cfg.Registry == nil || cfg.modFile == nil || cfg.modFile.Module == "" {
		return nil, nil
	}
	mainModPath := cfg.modFile.QualifiedModule()
	reqs := modrequirements.NewRequirements(
		mainModPath,
		cfg.Registry,
		cfg.modFile.DepVersions(),
		cfg.modFile.DefaultMajorVersions(),
	)
	mainModLoc := module.SourceLoc{
		FS:  cfg.fileSystem.ioFS(cfg.ModuleRoot),
		Dir: ".",
	}
	pkgPaths := make(map[string]bool)
	// Add any packages specified directly on the command line.
	for _, pkg := range pkgs {
		pkgPaths[pkg.resolvedCanonical] = true
	}
	// Add any imports found in other files.
	for _, f := range otherFiles {
		if f.Encoding != build.CUE {
			// not a CUE file; assume it has no imports for now.
			continue
		}
		syntax, err := cfg.fileSystem.getCUESyntax(f)
		if err != nil {
			return nil, fmt.Errorf("cannot get syntax for %q: %w", f.Filename, err)
		}
		for _, imp := range syntax.Imports {
			pkgPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				// Should never happen.
				return nil, fmt.Errorf("invalid import path %q in %s", imp.Path.Value, f.Filename)
			}
			// Canonicalize the path.
			pkgPath = module.ParseImportPath(pkgPath).Canonical().String()
			pkgPaths[pkgPath] = true
		}
	}
	// TODO use maps.Keys when we can.
	pkgPathSlice := make([]string, 0, len(pkgPaths))
	for p := range pkgPaths {
		pkgPathSlice = append(pkgPathSlice, p)
	}
	sort.Strings(pkgPathSlice)
	return modpkgload.LoadPackages(
		ctx,
		cfg.Module,
		mainModLoc,
		reqs,
		cfg.Registry,
		pkgPathSlice,
		func(pkgPath string, mod module.Version, fsys fs.FS, mf modimports.ModuleFile) bool {
			if !cfg.Tools && strings.HasSuffix(mf.FilePath, "_tool.cue") {
				return false
			}
			isTest := strings.HasSuffix(mf.FilePath, "_test.cue")
			var tagIsSet func(string) bool
			if mod.Path() == mainModPath {
				// In the main module.
				if isTest && !cfg.Tests {
					return false
				}
				tagIsSet = tg.tagIsSet
			} else {
				// Outside the main module.
				if isTest {
					// Don't traverse test files outside the main module
					return false
				}
				// Treat all build tag keys as unset.
				tagIsSet = func(string) bool {
					return false
				}
			}
			if err := shouldBuildFile(mf.Syntax, tagIsSet); err != nil {
				// Later build logic should pick up and report the same error.
				return false
			}
			return true
		},
	), nil
}
