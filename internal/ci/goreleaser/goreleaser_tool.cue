package goreleaser

import (
	"encoding/yaml"
	"path"
	"strings"

	"tool/file"
	"tool/exec"
	"tool/os"
	"tool/cli"
)

// _releaseTagPrefix is the prefix every release tag carries.
_releaseTagPrefix: "v"

command: release: {
	env: os.Environ

	let _env = env

	// The release being built, named by whoever dispatched the
	// release workflow. Empty tests a snapshot release instead: one
	// built in full and published nowhere. The job does not run at
	// the tag ref, so the environment it runs in says nothing about
	// which release this is.
	let _releaseVersion = *env.CUE_RELEASE_VERSION | ""

	tempDir: file.MkdirTemp & {
		path: string
	}

	goMod: file.Create & {
		contents: "module mod.test"
		filename: path.Join([tempDir.path, "go.mod"])
	}

	latestCUE: exec.Run & {
		env: {
			_env

			GOPROXY: "direct" // skip proxy.golang.org in case its @latest is lagging behind
		}
		$after: goMod
		dir:    tempDir.path
		cmd: ["go", "list", "-m", "-f", "{{.Version}}", "cuelang.org/go@latest"]
		stdout: string
	}

	let latestCUEVersion = strings.TrimSpace(latestCUE.stdout)

	tidyUp: file.RemoveAll & {
		$after: latestCUE
		path:   tempDir.path
	}

	cueModRoot: exec.Run & {
		cmd: ["go", "list", "-m", "-f", "{{.Dir}}", "cuelang.org/go"]
		stdout: string
	}

	let goreleaserCmd = [
		"goreleaser", "release", "-f", "-", "--clean",

		// Only release for real when a release was named; otherwise
		// test a snapshot release, which publishes nothing.
		//
		// TODO: Once there is a "goreleaser test" command,
		// switch to that instead of our workaround via "goreleaser release --snapshot".
		// See: https://github.com/goreleaser/goreleaser/issues/2355
		if !strings.HasPrefix(_releaseVersion, _releaseTagPrefix) {
			"--snapshot"
		},
	]
	let goreleaserConfigYAML = yaml.Marshal(config & {
		#latest: _releaseVersion == latestCUEVersion
	})

	info: cli.Print & {
		text: """
			latest CUE version: \(latestCUEVersion)
			release version: \(_releaseVersion)
			goreleaser cmd: \(strings.Join(goreleaserCmd, " "))

			goreleaser config yaml, indented for readability:
			  \(strings.Replace(goreleaserConfigYAML, "\n", "\n  ", -1))
			"""
	}

	goreleaser: exec.Run & {
		$after: info

		// Set the goreleaser configuration to be stdin
		stdin: goreleaserConfigYAML

		// Run at the root of the module
		dir: strings.TrimSpace(cueModRoot.stdout)

		cmd: goreleaserCmd
	}
}
