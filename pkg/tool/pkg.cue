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

// This file is maintained by hand: it is the authoritative description
// of the package's API for tooling such as editor completion and hover.
// Its consistency with the package is checked by the tests in
// cuelang.org/go/pkg.

// Package tool defines stateful operation types for cue commands.
//
// This package is only visible in cue files with a _tool.cue or
// _tool_test.cue ending. It cannot be imported as a regular package;
// its declarations form the schema of the top-level "command" map of
// tool files.
package tool

// A Command specifies a user-defined command.
Command: {
	// Tasks specifies the things to run to complete a command. Tasks
	// are typically underspecified and completed by the particular
	// internal handler that is running them. Tasks can be a single
	// task, or a full hierarchy of tasks.
	//
	// Tasks that depend on the output of other tasks are run after
	// such tasks. Use `$after` if a task needs to run after another
	// task but does not otherwise depend on its output.
	Tasks

	// $usage summarizes how a command takes arguments.
	//
	// Example:
	//     mycmd [-n] names
	$usage?: string

	// $short is short description of what the command does.
	$short?: string

	// $long is a longer description that spans multiple lines and
	// likely contain examples of usage of the command.
	$long?: string
}

// Tasks defines a hierarchy of tasks. A command completes if all
// tasks have run to completion.
Tasks: Task | {
	[Name]: Tasks
}

// Name defines a valid task or command name.
Name: =~#"^\PL([-](\PL|\PN))*$"#

// A Task defines a step in the execution of a command.
Task: {
	// $id indicates the operation to run. Do not use this field
	// directly; instead unify with a task imported from one of the
	// tool packages.
	$id: =~#"\."#

	// $after can be used to specify a task is run after another one,
	// when it does not otherwise refer to an output of that task.
	$after?: Task | [...Task]
}
