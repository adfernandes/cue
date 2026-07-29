// Copyright 2026 The CUE Authors
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

package cache

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/internal/golangorgx/gopls/protocol"
	"cuelang.org/go/internal/golangorgx/tools/diff"
	"cuelang.org/go/internal/pretty"
)

// CodeActionOrganizeImports calculates the edits needed to organise
// imports:
//
//  1. Imports which are not used in the file are removed;
//  2. All import declarations are merged into a single declaration at
//     the location of the first, sorted lexicographically by import
//     path as one flat list;
//  3. The organized imports region is constructed via the AST and
//     rendered by cue/format, so its layout is canonical by
//     construction; the rest of the file is untouched.
func (w *Workspace) CodeActionOrganizeImports(ctx context.Context, params *protocol.CodeActionParams, delayEdit bool) (*protocol.WorkspaceEdit, error) {
	fileUri := params.TextDocument.URI
	f, fe, mapper, err := w.FileEvaluatorForURI(fileUri, LoadAll)
	if f == nil || fe == nil || mapper == nil || err != nil {
		return nil, err
	}

	content := f.tokFile.Content()

	// Organize only when the entire buffer parses cleanly: usage
	// analysis over a partial parse could classify a used import as
	// unused and silently delete it.
	if fh, err := w.overlayFS.ReadFile(fileUri); err != nil {
		return nil, nil
	} else if _, _, err := fh.ReadCUE(standaloneParserConfig); err != nil {
		return nil, nil
	}

	if delayEdit {
		return &protocol.WorkspaceEdit{}, nil
	}

	used := func(spec *ast.ImportSpec) bool {
		pos := spec.Path.Pos()
		if spec.Name != nil {
			pos = spec.Name.Pos()
		}
		return len(fe.UsagesForOffset(pos.Offset(), false)) > 0
	}

	organized := organizeImports(f.syntax, content, extractLineEnding(content, f.tokFile.Lines()), used)
	if organized == nil {
		organized = content
	}

	diffEdits := diff.Strings(string(content), string(organized))
	edits, err := protocol.EditsFromDiffEdits(f.mapper, diffEdits)
	if err != nil {
		return nil, nil
	}

	docChanges := protocol.TextEditsToDocumentChanges(params.TextDocument.URI, f.tokFile.Revision(), edits)
	return &protocol.WorkspaceEdit{DocumentChanges: docChanges}, nil
}

// organizeImports computes the organized whole-file content, or nil
// when the file has no import declarations. parsed is the file's
// cached AST: it is shared with every other consumer of the file and
// is never mutated here — the construction clones every node it
// modifies. used reports whether an import spec has any usage.
//
// The imports region — from the first declaration's first token (or
// its doc comment) to the last declaration's closing token, extended
// over same-line trailing comments — is replaced by a rendering of the
// merged declaration; all bytes outside the region are preserved
// verbatim.
func organizeImports(parsed *ast.File, content []byte, lineEnding string, used func(*ast.ImportSpec) bool) []byte {
	decls := slices.Collect(parsed.ImportDecls())
	if len(decls) == 0 {
		return nil
	}

	regionStart, regionEnd := importsRegion(decls)

	// Survivors, in source order. Comments attached directly to a
	// removed import are removed with it.
	var kept []*ast.ImportSpec
	for _, d := range decls {
		for _, s := range d.Specs {
			if used(s) {
				kept = append(kept, s)
			}
		}
	}

	prefix := content[:regionStart]
	suffix := content[regionEnd:]

	parts := buildOrganizedDecls(decls, kept, regionEnd)

	if len(parts) == 0 {
		// Every import is removed and no comments survive: the region
		// disappears entirely, leaving no residual blank lines.
		prefix = bytes.TrimRight(prefix, "\r\n")
		suffix = bytes.TrimLeft(suffix, "\r\n\t ")
		switch {
		case len(prefix) == 0:
			return suffix
		case len(suffix) == 0:
			return append(slices.Clip(prefix), lineEnding...)
		default:
			out := slices.Clip(prefix)
			out = append(out, lineEnding...)
			out = append(out, lineEnding...)
			return append(out, suffix...)
		}
	}

	// Render via a synthetic file: file rendering wraps each
	// declaration with its own attached comment groups (doc comments,
	// trailing comments), which bare-declaration rendering does not,
	// and lays out standalone comment paragraphs between declarations.
	rendered, err := format.Node(&ast.File{Decls: parts})
	if err != nil {
		return nil
	}
	rendered = bytes.TrimRight(rendered, "\n")
	if lineEnding != "\n" {
		rendered = bytes.ReplaceAll(rendered, []byte("\n"), []byte(lineEnding))
	}

	out := make([]byte, 0, len(prefix)+len(rendered)+len(suffix))
	out = append(out, prefix...)
	out = append(out, rendered...)
	out = append(out, suffix...)
	return out
}

// importsRegion returns the byte range of the imports region: from
// the first declaration's first token (or its doc comment, which
// renders as part of the merged declaration) through the last
// declaration's closing token, extended over any comment group
// trailing that token on the same line. Detached comment groups on
// later lines are outside the region even when the parser attached
// them to an import node; buildOrganizedDecls drops those from the
// cloned nodes so their (preserved) suffix bytes are not duplicated.
func importsRegion(decls []*ast.ImportDecl) (start, end int) {
	first, last := decls[0], decls[len(decls)-1]

	start = first.Pos().Offset()
	for _, cg := range ast.Comments(first) {
		if cg.Position == pretty.PosDoc {
			start = min(start, cg.Pos().Offset())
		}
	}

	if last.Rparen.IsValid() {
		end = last.Rparen.Offset() + len(")")
	} else {
		for _, s := range last.Specs {
			end = max(end, s.Path.End().Offset())
		}
	}
	extend := func(n ast.Node) {
		for _, cg := range ast.Comments(n) {
			if cg.Line && cg.Pos().Offset() >= end {
				end = max(end, cg.End().Offset())
			}
		}
	}
	extend(last)
	for _, s := range last.Specs {
		extend(s)
	}
	return start, end
}

// buildOrganizedDecls constructs the declarations that replace the
// imports region: any standalone comment paragraphs that sat outside
// the parenthesized blocks (before the merged declaration, in source
// order), followed by the single merged import declaration. It returns
// nil when nothing survives (no imports kept and no comments to
// preserve).
//
// The kept specs are sorted lexicographically by import path (stable,
// so duplicate paths keep their source order). Comment groups are
// re-placed by their source position, never by raw parser attachment
// (the parser attaches detached groups backward to the preceding
// spec, not forward as the placement rules require):
//
//   - a comment directly attached to an import (doc immediately above,
//     or trailing on its line) travels - or is removed - with it;
//   - doc comments of the merged declarations concatenate, in source
//     order, into the merged declaration's doc comment, with an empty
//     comment line separating the original groups;
//   - a detached (blank-line-separated) group inside a block belongs
//     to the import that follows it in that block and moves with it;
//     when that import is removed it re-homes to the next surviving
//     import of the block, or collects at the end of the declaration
//     when none follows - it never crosses the closing parenthesis;
//   - a comment on the hosting declaration's parentheses stays on the
//     merged declaration's parentheses; a merged-away declaration's
//     closing-parenthesis comment collects after the merged
//     declaration, and its opening-parenthesis comment becomes a
//     leading comment of that block's first surviving import;
//   - detached paragraphs outside the blocks concatenate in front of
//     the merged declaration, in source order.
//
// A single kept spec produces the parenless single-line form, hoisting
// doc comments above the import keyword.
//
// The input declarations belong to the file's cached AST, which is
// shared with every other consumer of the file and is never mutated:
// the kept specs and every re-placed comment group are cloned first.
func buildOrganizedDecls(decls []*ast.ImportDecl, kept []*ast.ImportSpec, regionEnd int) []ast.Decl {
	keptSet := make(map[*ast.ImportSpec]bool, len(kept))
	clone := make(map[*ast.ImportSpec]*ast.ImportSpec, len(kept))
	for _, s := range kept {
		keptSet[s] = true
		clone[s] = cloneImportSpec(s)
	}

	// blockOf reports which declaration's parentheses enclose a byte
	// offset, or -1 when the offset lies between declarations.
	blockOf := func(off int) int {
		for i, d := range decls {
			if d.Lparen.IsValid() && d.Rparen.IsValid() &&
				off > d.Lparen.Offset() && off < d.Rparen.Offset() {
				return i
			}
		}
		return -1
	}

	type placed struct {
		cg    *ast.CommentGroup // a clone, free to modify
		block int
		off   int
	}
	var docGroups []*ast.CommentGroup // doc groups merging, in source order
	var hostParen []*ast.CommentGroup // host `(` / `)` groups, kept as-is
	var afterDecl []*ast.CommentGroup // merged-away `)` groups
	var detached []placed             // in-block detached groups
	var front []placed                // paragraphs outside blocks

	addDetached := func(cg *ast.CommentGroup, off int) {
		if b := blockOf(off); b >= 0 {
			detached = append(detached, placed{cg, b, off})
		} else {
			front = append(front, placed{cg, -1, off})
		}
	}

	for i, d := range decls {
		for _, cg := range ast.Comments(d) {
			off := cg.Pos().Offset()
			if off >= regionEnd {
				continue
			}
			cg = cloneCommentGroup(cg)
			switch {
			case cg.Position == pretty.PosDoc && cg.Doc:
				docGroups = append(docGroups, cg)
			case cg.Line && cg.Position == pretty.PosSuffix:
				if i == 0 {
					// The hosting declaration's `import ( // c` group
					// stays on the opening parenthesis.
					hostParen = append(hostParen, cg)
				} else {
					// A merged-away `import ( // c` group stays inside
					// the block, becoming a leading comment of the
					// block's first surviving import.
					cg.Line = false
					ast.SetRelPos(cg, token.Newline)
					addDetached(cg, off)
				}
			case cg.Line && cg.Position >= pretty.PosTrailingMin:
				if i == 0 {
					hostParen = append(hostParen, cg)
				} else {
					// A merged-away `) // c` group never crosses
					// inward; it collects after the merged declaration
					// on its own line.
					cg.Line = false
					ast.SetRelPos(cg, token.Newline)
					afterDecl = append(afterDecl, cg)
				}
			default:
				addDetached(cg, off)
			}
		}
		for _, spec := range d.Specs {
			var direct []*ast.CommentGroup
			for _, cg := range ast.Comments(spec) {
				off := cg.Pos().Offset()
				if off >= regionEnd {
					// Textually after the imports: the group's bytes
					// are preserved in the suffix; leaving it off the
					// clone avoids rendering it twice.
					continue
				}
				if (cg.Position == pretty.PosDoc && cg.Doc) || cg.Line {
					// Directly attached: travels - or is removed -
					// with its import.
					if keptSet[spec] {
						direct = append(direct, cloneCommentGroup(cg))
					}
					continue
				}
				// Detached: the parser attached it backward; re-place
				// it by source position.
				addDetached(cloneCommentGroup(cg), off)
			}
			if c, ok := clone[spec]; ok {
				ast.SetComments(c, direct)
			}
		}
	}

	slices.SortStableFunc(detached, func(a, b placed) int { return cmp.Compare(a.off, b.off) })
	slices.SortStableFunc(front, func(a, b placed) int { return cmp.Compare(a.off, b.off) })

	// Authored leading separation of every spec (kept or removed),
	// read before the flat-list normalization below.
	authoredRel := make(map[*ast.ImportSpec]token.RelPos)
	for _, d := range decls {
		for _, s := range d.Specs {
			authoredRel[s] = pretty.LeadingRelPos(s)
		}
	}

	slices.SortStableFunc(kept, func(a, b *ast.ImportSpec) int {
		return cmp.Compare(a.Path.Value, b.Path.Value)
	})

	// Forward association: a detached group belongs to the import that
	// follows it in its block; with none surviving there, it collects
	// at the end of the merged declaration. The separation between the
	// group and the import it lands on pairs with the group's original
	// follower, so re-homing past removed imports keeps the authored
	// paragraph spacing.
	forwarded := make(map[*ast.ImportSpec][]*ast.CommentGroup)
	pairedRel := make(map[*ast.ImportSpec]token.RelPos)
	var endGroups []*ast.CommentGroup
	for _, pd := range detached {
		var target, follower *ast.ImportSpec
		for _, spec := range decls[pd.block].Specs {
			if spec.Pos().Offset() <= pd.off {
				continue
			}
			if follower == nil {
				follower = spec
			}
			if keptSet[spec] {
				target = spec
				break
			}
		}
		if target == nil {
			endGroups = append(endGroups, pd.cg)
			continue
		}
		pd.cg.Position = pretty.PosDoc
		pd.cg.Doc = false
		pd.cg.Line = false
		forwarded[target] = append(forwarded[target], pd.cg)
		pairedRel[target] = authoredRel[follower]
	}

	// Flatten the merged list: every spec starts on a plain new line.
	// A spec receiving forwarded paragraphs keeps the groups' authored
	// spacing instead: the paragraph's own separation above, and the
	// original group-to-import separation below (unless the spec's own
	// doc comment already provides it).
	for _, spec := range kept {
		c := clone[spec]
		groups := forwarded[spec]
		if len(groups) == 0 {
			setSpecRelPos(c, token.Newline)
			continue
		}
		own := ast.Comments(c)
		hasOwnDoc := pretty.HasDocComment(c)
		ast.SetComments(c, append(groups, own...))
		if !hasOwnDoc {
			rel := pairedRel[spec]
			if c.Name != nil {
				c.Name.NamePos = c.Name.NamePos.WithRel(rel)
			} else {
				c.Path.ValuePos = c.Path.ValuePos.WithRel(rel)
			}
		}
	}

	merged := &ast.ImportDecl{}

	if len(kept) == 1 {
		// Single-line form: doc comments (own and forwarded) hoist
		// above the import keyword.
		c := clone[kept[0]]
		cgs := ast.Comments(c)
		var rest []*ast.CommentGroup
		for _, cg := range cgs {
			if cg.Position == pretty.PosDoc {
				docGroups = append(docGroups, cg)
			} else {
				rest = append(rest, cg)
			}
		}
		ast.SetComments(c, rest)
		setSpecRelPos(c, token.NoRelPos)
	} else if len(kept) > 1 && len(endGroups) > 0 {
		// End-of-block groups anchor after the final spec, inside the
		// closing parenthesis (they never cross it).
		lastSpec := clone[kept[len(kept)-1]]
		for _, cg := range endGroups {
			cg.Position = pretty.PosSuffix
			cg.Line = false
			cg.Doc = false
			ast.AddComment(lastSpec, cg)
		}
		endGroups = nil
	}

	merged.Specs = make([]*ast.ImportSpec, len(kept))
	for i, spec := range kept {
		merged.Specs[i] = clone[spec]
	}

	if len(docGroups) > 0 {
		slices.SortStableFunc(docGroups, func(a, b *ast.CommentGroup) int {
			return cmp.Compare(a.Pos().Offset(), b.Pos().Offset())
		})
		var docList []*ast.Comment
		for i, cg := range docGroups {
			if i > 0 {
				// An empty comment line separates the merged groups.
				docList = append(docList, &ast.Comment{Text: "//"})
			}
			docList = append(docList, cg.List...)
		}
		doc := &ast.CommentGroup{Doc: true, List: docList}
		ast.SetRelPos(doc, token.NewSection)
		ast.AddComment(merged, doc)
	}
	for _, cg := range hostParen {
		ast.AddComment(merged, cg)
	}
	// End groups with no spec to anchor to (single or zero survivors)
	// land after the declaration, keeping their authored separation,
	// rather than being dropped.
	for _, cg := range endGroups {
		cg.Position = pretty.PosImportAfterRparen
		cg.Line = false
		cg.Doc = false
		if len(kept) > 0 {
			ast.AddComment(merged, cg)
		} else {
			front = append(front, placed{cg, -1, cg.Pos().Offset()})
		}
	}
	for _, cg := range afterDecl {
		cg.Position = pretty.PosImportAfterRparen
		ast.AddComment(merged, cg)
	}

	var parts []ast.Decl
	if len(front) > 0 {
		slices.SortStableFunc(front, func(a, b placed) int { return cmp.Compare(a.off, b.off) })
		for _, pf := range front {
			pf.cg.Position = pretty.PosDoc
			pf.cg.Doc = false
			pf.cg.Line = false
			ast.SetRelPos(pf.cg, token.NewSection)
			parts = append(parts, pf.cg)
		}
	}
	if len(kept) > 0 {
		parts = append(parts, merged)
	}
	return parts
}

// cloneImportSpec returns a copy of s with no comments attached; the
// caller attaches cloned comment groups as needed.
func cloneImportSpec(s *ast.ImportSpec) *ast.ImportSpec {
	out := &ast.ImportSpec{EndPos: s.EndPos}
	if s.Name != nil {
		out.Name = &ast.Ident{NamePos: s.Name.NamePos, Name: s.Name.Name}
	}
	if s.Path != nil {
		out.Path = &ast.BasicLit{ValuePos: s.Path.ValuePos, Kind: s.Path.Kind, Value: s.Path.Value}
	}
	return out
}

// cloneCommentGroup returns a copy of cg whose flags, position, and
// comments can be freely modified.
func cloneCommentGroup(cg *ast.CommentGroup) *ast.CommentGroup {
	list := make([]*ast.Comment, len(cg.List))
	for i, c := range cg.List {
		list[i] = &ast.Comment{Slash: c.Slash, Text: c.Text}
	}
	return &ast.CommentGroup{Doc: cg.Doc, Line: cg.Line, Position: cg.Position, List: list}
}

// setSpecRelPos sets the leading relative position of an import spec:
// its first token, or its doc comment when one is attached (the doc
// comment then owns the spec's leading separation).
func setSpecRelPos(s *ast.ImportSpec, r token.RelPos) {
	if cg := pretty.FirstCommentAt(s, pretty.PosDoc); cg != nil {
		ast.SetRelPos(cg, r)
		return
	}
	pos := s.Pos().WithRel(r)
	if s.Name != nil {
		s.Name.NamePos = pos
	} else {
		s.Path.ValuePos = pos
	}
}
