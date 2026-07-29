package workspace

import (
	"slices"
	"strings"
	"testing"

	"cuelang.org/go/internal/golangorgx/gopls/protocol"
	I "cuelang.org/go/internal/golangorgx/gopls/test/integration"
	"github.com/go-quicktest/qt"
)

func TestCodeActionOrganizeImports(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected string
		// check, when set, is used to validate the resulting buffer
		// instead of comparing it to expected.
		check func(t *testing.T, after string)
		// actionOptional tolerates the Organize Imports code action
		// not being offered; the buffer is then unchanged.
		actionOptional bool
	}

	// Body used by the outside_region_untouched case: deliberately
	// badly formatted, so its verbatim survival shows that organizing
	// does not reformat bytes outside the imports region.
	outsideBody := `
y: {
    z:   42    // body comment
	}

x:p2&p3
`
	// A file that does not parse cleanly in its entirety is not
	// organized: usage analysis over a partial parse could classify a
	// used import as unused and delete it. The code action is not
	// offered, leaving the buffer unchanged.
	parseErrorInput := `
package p1

import (
	"mod.com/p3"
	"mod.com/p2"
)

x: p3 &
`[1:]

	testCases := []testCase{
		{
			name:     "empty",
			input:    "package p1\n",
			expected: "package p1\n",
		},

		{
			name: "used_single",
			input: `
package p1

import "mod.com/p2"

x: p2
`[1:],
			expected: `
package p1

import "mod.com/p2"

x: p2
`[1:],
		},

		{
			name: "used_multiple_separate",
			input: `
package p1

import "mod.com/p3"
import "mod.com/p2"

x: p2 & p3
`[1:],
			expected: `
package p1

import "mod.com/p3"
import "mod.com/p2"

x: p2 & p3
`[1:],
		},

		{
			name: "used_multiple_joined",
			input: `
package p1

import (
  "mod.com/p3"
  	  	 "mod.com/p2"
              )

x: p2 & p3
`[1:],
			expected: `
package p1

import (
	"mod.com/p2"
	"mod.com/p3"
)

x: p2 & p3
`[1:],
		},

		{
			name: "mixed_separate",
			input: `
package p1

import "mod.com/p3"
import "mod.com/p2"

x: p3
`[1:],
			expected: `
package p1

import "mod.com/p3"


x: p3
`[1:],
		},

		{
			name: "mixed_joined_one_survives",
			input: `
package p1

import (
"mod.com/p3"
      "mod.com/p2")

x: p3
`[1:],
			expected: `
package p1

import "mod.com/p3"

x: p3
`[1:],
		},

		{
			name: "mixed_joined_several_survive",
			input: `
package p1

import (
"mod.com/p3"
"mod.com/p4"
      "mod.com/p2")

x: p4 & p2
`[1:],
			expected: `
package p1

import (
	"mod.com/p2"
	"mod.com/p4"
)

x: p4 & p2
`[1:],
		},

		{
			name: "mixed_mixed",
			input: `
package p1

import (
"mod.com/p3"
"mod.com/p2"
)

import "mod.com/p1"

import (
"mod.com/p4"
"mod.com/p7"
      "mod.com/p6")

import "mod.com/p5"

x: p1 & p4 & p7
`[1:],
			expected: `
package p1



import "mod.com/p1"

import (
	"mod.com/p4"
	"mod.com/p7"
)



x: p1 & p4 & p7
`[1:],
		},

		{
			// Comments attached to surviving specs, and comments
			// within the decl (before the closing paren), are
			// preserved. Comments attached to a removed spec are
			// removed with it.
			name: "comments_preserved",
			input: `
package p1

import (
	// doc for p3
	"mod.com/p3" // pinned
	// doc for unused p2
	"mod.com/p2"
	// floating comment for p4
	"mod.com/p4"
	// before the closing paren
)

x: p3 & p4
`[1:],
			expected: `
package p1

import (
	// doc for p3
	"mod.com/p3" // pinned
	// floating comment for p4
	"mod.com/p4"
	// before the closing paren
)

x: p3 & p4
`[1:],
		},

		{
			// With a single surviving import, its doc comment moves
			// above the import keyword and its line comment stays on
			// the line.
			name: "comments_preserved_single_survivor",
			input: `
package p1

import (
	// doc for p3
	"mod.com/p3" // pinned
	"mod.com/p2"
)

x: p3
`[1:],
			expected: `
package p1

// doc for p3
import "mod.com/p3" // pinned

x: p3
`[1:],
		},

		{
			name: "comment_presence",
			input: `
package p1

// decl doc comment
import (
	// p3 doc comment
	"mod.com/p3" // p3 trailing

	// between the specs

	"mod.com/p2"
)

// between decls

import "mod.com/p4" // p4 trailing

x: p2 & p3 & p4
`[1:],
			check: func(t *testing.T, after string) {
				for _, comment := range []string{
					"// decl doc comment",
					"// p3 doc comment",
					"// p3 trailing",
					"// between the specs",
					"// between decls",
					"// p4 trailing",
				} {
					qt.Check(t, qt.StringContains(after, comment))
				}
			},
		},

		{
			name:  "crlf_not_mixed",
			input: "package p1\r\n\r\nimport (\r\n\t\"mod.com/p3\"\r\n\t\"mod.com/p2\"\r\n)\r\n\r\nx: p2 & p3\r\n",
			check: func(t *testing.T, after string) {
				qt.Check(t, qt.Equals(strings.Count(after, "\n"), strings.Count(after, "\r\n")),
					qt.Commentf("organized buffer mixes line endings: %q", after))
			},
		},

		{
			name: "outside_region_untouched",
			input: `
package p1

import (
	"mod.com/p3"
	"mod.com/p2"
)
`[1:] + outsideBody,
			check: func(t *testing.T, after string) {
				qt.Check(t, qt.StringContains(after, outsideBody))
				qt.Check(t, qt.Equals(strings.HasPrefix(after, "package p1\n"), true))
			},
		},

		{
			name:           "parse_error_gate",
			input:          parseErrorInput,
			expected:       parseErrorInput,
			actionOptional: true,
		},
	}

	for _, tc := range testCases {
		fun := func(t *testing.T, env *I.Env) {
			resolveSupport, err := env.Editor.EditResolveSupport()
			if err != nil {
				t.Fatal(err)
			}

			env.OpenFile("input.cue")
			env.Await(env.DoneWithOpen())
			rootURI := env.Sandbox.Workdir.RootURI()

			cursor := protocol.Location{URI: rootURI + "/input.cue"}

			actions, err := env.Editor.CodeAction(env.Ctx, cursor, nil)
			if err != nil {
				qt.Assert(t, qt.IsNil(err))
			}

			var action protocol.CodeAction
			found := slices.ContainsFunc(actions, func(a protocol.CodeAction) bool {
				if a.Title == "Organize Imports" {
					action = a
					return true
				}
				return false
			})
			if !found && !tc.actionOptional {
				t.Fatal("Failed to find Organize Imports code action")
			}
			if found {
				// If we advertised to the LSP that we support lazy
				// resolution for codeactions, we should have been sent
				// back a nil-Edit property.
				qt.Assert(t, qt.Equals(action.Edit == nil, resolveSupport))
				// Calling ApplyCodeAction will make the additional call
				// to resolve the Edit property if necessary.
				env.ApplyCodeAction(action)
			}
			after := env.BufferText("input.cue")
			if tc.check != nil {
				tc.check(t, after)
			} else {
				qt.Check(t, qt.Equals(after, tc.expected))
			}
		}

		t.Run(tc.name+"/eager", func(t *testing.T) {
			I.WithOptions(I.RootURIAsDefaultFolder()).Run(t, "-- input.cue --\n"+tc.input, fun)
		})

		t.Run(tc.name+"/lazy", func(t *testing.T) {
			I.WithOptions(
				I.RootURIAsDefaultFolder(),
				I.CapabilitiesJSON([]byte(`{
  "textDocument": {"codeAction": {
    "dataSupport": true,
    "resolveSupport": {"properties": ["edit"]}
  }}
}`)),
			).Run(t, "-- input.cue --\n"+tc.input, fun)
		})
	}
}
