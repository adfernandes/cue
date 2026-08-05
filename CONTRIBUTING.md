# Contribution Guide

There are many ways to contribute to CUE without writing code!

* Ask or answer questions via GitHub discussions, Slack, and Discord
* Raise issues such as bug reports or feature requests on GitHub
* Contributing thoughts and use cases to proposals. CUE can be and is
  being used in many varied different ways. Sharing experience reports helps
to shape proposals and designs.
* Create content: share blog posts, tutorials, videos, meetup talks, etc
* Add your project to [Unity](https://cue.dev/products/unity/) to help us test changes to CUE

## Before contributing code

As with many open source projects, CUE uses the GitHub [issue
tracker](https://github.com/cue-lang/cue/issues) to not only track bugs, but
also coordinate work on new features, bugs, designs and proposals.  Given the
inherently distributed nature of open source this coordination is important
because it very often serves as the main form of communication between
contributors.

You can also exchange ideas or feedback with other contributors via the
`#contributing` [Slack channel](https://cuelang.slack.com/archives/CMY132JKY),
as well as the contributor office hours calls which we hold via the
[community calendar](https://cuelang.org/s/community-calendar) once per week.

### Check the issue tracker

Whether you already know what contribution to make, or you are searching for an
idea, the [issue tracker](https://cuelang.org/issues) is always the first place
to go.  Issues are triaged to categorize them and manage the workflow.

Most issues will be marked with one of the following workflow labels (links are
to queries in the issue tracker):

- [**Triage**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3ATriage):
  Requires review by one of the core project maintainers.
- [**NeedsInvestigation**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3ANeedsInvestigation):
  The issue is not fully understood and requires analysis to understand the root
cause.
- [**NeedsDecision**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3ANeedsDecision):
  the issue is relatively well understood, but the CUE team hasn't yet decided
  the best way to address it.  It would be better to wait for a decision before
  writing code.  If you are interested on working on an issue in this state, feel
  free to "ping" maintainers in the issue's comments if some time has passed
  without a decision.
- [**NeedsFix**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3ANeedsFix):
  the issue is fully understood and code can be written to fix it.
- [**help wanted**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3A"help+wanted"):
  project maintainers need input from someone who has experience or expertise to
  answer or progress this issue.
- [**good first issue**](https://cuelang.org/issues?q=is%3Aissue+is%3Aopen+label%3A"good+first+issue"):
  often combined with `NeedsFix`, `good first issue` indicates an issue is very
  likely a good candidate for someone
  looking to make their first code contribution.

### Open an issue for any new problem

Excluding very trivial changes, all contributions should be connected to an
existing issue.  Feel free to open one and discuss your plans.  This process
gives everyone a chance to validate the design, helps prevent duplication of
effort, and ensures that the idea fits inside the goals for the language and
tools.  It also checks that the design is sound before code is written; the code
review tool is not the place for high-level discussions.

## Becoming a code contributor

Code contributions to the CUE project are made via [GitHub Pull
Requests](https://docs.github.com/en/pull-requests). We assume you have a basic
understanding of [`git`](https://git-scm.com/) and [Go](https://go.dev/)
(1.24 or later).

Historically the CUE project also accepted contributions via GerritHub, with
GerritHub acting as the source of truth. The GerritHub repository is now
read-only: links to past code reviews (such as `https://cuelang.org/cl/NNN`)
continue to work, but no new changes can be sent or merged there. All
contributions happen via GitHub Pull Requests.

Contributions must be accompanied by a Developer Certificate of Origin.

### Asserting a Developer Certificate of Origin

Contributions to the CUE project must be accompanied by a [Developer Certificate
of Origin](https://developercertificate.org/), we are using version 1.1.

All commit messages must contain the `Signed-off-by` line with an email address
that matches the commit author. This line asserts the Developer Certificate of Origin.

When committing, use the `--signoff` (or `-s`) flag:

```console
$ git commit -s
```

You can also [set up a prepare-commit-msg git
hook](#do-i-really-have-to-add-the--s-flag-to-each-commit) to not have to supply
the `-s` flag.

The explanation of the contribution workflow that follows assumes all commits
you create are signed-off in this way.

## Preparing for GitHub Pull Request (PR) Contributions

First-time contributors that are already familiar with the <a
href="https://docs.github.com/get-started/quickstart/github-flow">GitHub flow</a> are
encouraged to use the same process for CUE contributions.

Here is a checklist of the steps to follow when contributing via GitHub PR
workflow:

- **Step 0**: Review the guidelines on [Good Commit Messages](#good-commit-messages),
  [The Review Process](#the-review-process) and [Miscellaneous Topics](#miscellaneous-topics)
- **Step 1**: Create a GitHub account if you do not have one.
- **Step 2**: [Fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/fork-a-repo)
  the CUE project, and clone your fork locally

That's it! You are now ready to send a change via GitHub, the subject of the
next section.

## Sending a change via GitHub

The GitHub documentation around [working with
forks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/getting-started/about-collaborative-development-models)
is extensive so we will not cover that ground here.

Before making any changes it's a good idea to verify that you have a stable
baseline by running the tests:

```console
$ go test ./...
```

Then make your planned changes and create a commit from the staged changes:

```console
# Edit files
$ git add file1 file2
$ git commit -s
```

Notice as we explained above, the `-s` flag asserts the Developer Certificate of
Origin by adding a `Signed-off-by` line to a commit. When writing a commit
message, remember the guidelines on [good commit messages](#good-commit-messages).

You've written and tested your code, but before sending code out for review, run
all the tests from the root of the repository to ensure the changes don't break
other packages or programs:

```console
$ go test ./...
```

Your change is now ready!
[Submit a PR](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request)
in the usual way.

Once your PR is submitted, a maintainer will trigger continuous integration (CI)
workflows to run and [review your proposed
change](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/reviewing-changes-in-pull-requests/reviewing-proposed-changes-in-a-pull-request).
The results from CI and the review might indicate further changes are required,
and this is where the CUE project differs from others:

### Making changes to a PR

Some projects accept and encourage multiple commits in a single PR. Either as a
way of breaking down the change into smaller parts, or simply as a record of the
various changes during the review process.

The CUE project treats a single commit as the unit of change. Therefore, all
PRs must only contain a single commit. This keeps each change self-contained,
and encourages unrelated changes to be broken into separate PRs. But how does
this work if you need to make changes requested during the review process? Does
this not require you to create additional commits?

The easiest way to maintain a single commit is to amend an existing commit.
Rather misleadingly, this doesn't actually amend a commit, but instead creates a
new commit which is the result of combining the last commit and any new changes:

```console
# PR is submitted, feedback received. Time to make some changes!

$ git add file1 file2   # stage the files we have added/removed/changed
$ git commit --amend    # amend the last commit
$ git push -f           # push the amended commit to your PR
```

The `-f` flag is required to force push your branch to GitHub: this overrides a
warning from `git` telling you that GitHub knows nothing about the relationship
between the original commit in your PR and the amended commit.

What happens if you accidentally create an additional commit and now have two
commits on your branch? No worries, you can "squash" commits on a branch to
create a single commit. See the GitHub documentation on [how to squash commits
with GitHub Desktop](https://docs.github.com/en/desktop/managing-commits/squashing-commits-in-github-desktop),
or using the [`git` command
interactively](https://medium.com/@slamflipstrom/a-beginners-guide-to-squashing-commits-with-git-rebase-8185cf6e62ec).

### PR approved!

With the review cycle complete, the CI checks green and your PR approved, a
maintainer will merge it into the `master` branch. Congratulations! You will
have made your first contribution to the CUE project.

## Good commit messages

Commit messages in CUE follow a specific set of conventions, which we discuss in
this section.

Here is an example of a good one:

```
cue/ast/astutil: fix resolution bugs

This fixes several bugs and documentation bugs in
identifier resolution.

1. Resolution in comprehensions would resolve identifiers
   to themselves.

2. Label aliases now no longer bind to references outside the scope
   of the field. The compiler would catch this invalid bind and
   report an error, but it is better not to bind in the first place.

3. Remove some more mentions of Template labels.

4. Documentation for comprehensions was incorrect
   (Scope and Node were reversed).

5. Aliases X in `X=[string]: foo` should only be visible in foo.

Fixes #946
```

### First line

The first line of the change description is conventionally a short one-line
summary of the change, prefixed by the primary affected package
(`cue/ast/astutil` in the example above).

A rule of thumb is that it should be written so to complete the sentence "This
change modifies CUE to \_\_\_\_." That means it does not start with a capital
letter, is not a complete sentence, and actually summarizes the result of the
change.

Follow the first line by a blank line.

### Main content

The rest of the description elaborates and should provide context for the change
and explain what it does.  Write in complete sentences with correct punctuation,
just like for your comments in CUE.  Don't use HTML, Markdown, or any other
markup language.

### Referencing issues

The special notation `Fixes #12345` associates the change with issue 12345 in
the [CUE issue tracker](https://cuelang.org/issue/12345) When this change is
eventually applied, the issue tracker will automatically mark the issue as
fixed.

If the change is a partial step towards the resolution of the issue, uses the
notation `Updates #12345`.  This will leave a comment in the issue linking back
to the change, but it will not close the issue when the change is applied.

All issues are tracked in the main repository's issue tracker.
If you are sending a change against a subrepository, you must use the
fully-qualified syntax supported by GitHub to make sure the change is linked to
the issue in the main repository, not the subrepository (eg. `Fixes cue-lang/cue#999`).

## The review process

This section explains the review process in detail and how to approach reviews
after a change has been sent for review.

### Common mistakes

When a PR is submitted, it is usually triaged within a few days.  A
maintainer will have a look and provide some initial review that for first-time
contributors usually focuses on basic cosmetics and common mistakes.  These
include things like:

- Commit message not following the suggested format.
- The lack of a linked GitHub issue.  The vast majority of changes require a
  linked issue that describes the bug or the feature that the change fixes or
  implements, and consensus should have been reached on the tracker before
  proceeding with it.  Code reviews do not discuss the merit of the change, just
  its implementation.  Only trivial or cosmetic changes will be accepted without
  an associated issue.

### Continuous Integration (CI) checks

After an initial reading of your change, maintainers will trigger CI checks,
that run a full test suite and [Unity](https://cue.dev/products/unity/)
checks.  Most CI tests complete in a few minutes, at which point the results
are presented as checks towards the bottom of the PR.

If any of the CI checks fail, follow the link and check the full logs.  Try to
understand what broke, update your change to fix it, and upload again.
Maintainers will trigger a new CI run to see if the problem was fixed.

### Reviews

The CUE community values very thorough reviews.  Think of each review comment
like a ticket: you are expected to somehow "close" it by acting on it, either by
implementing the suggestion or convincing the reviewer otherwise.

After you update the change, go through the review comments and make sure to
reply to every one.  You can mark a comment as resolved to indicate that you've
implemented the reviewer's suggestion; otherwise, click on "Reply" and explain
why you have not, or what you have done instead.

It is perfectly normal for changes to go through several round of reviews, with
one or more reviewers making new comments every time and then waiting for an
updated change before reviewing again.  This cycle happens even for experienced
contributors, so don't be discouraged by it.

### Review responses in GitHub

When reviewing a PR, a reviewer will indicate the nature of their response:

* **Comments** - general feedback without explicit approval.
* **Approve** - feedback and approval for this PR to be merged.
* **Request changes** - feedback that must be addressed before this PR can proceed.

### Merging an approved change

After the PR has been "Approved", a maintainer will merge it into the `master`
branch.

The two steps (approving and merging) are separate because in some cases
maintainers may want to approve a change but not merge it right away (for
instance, the tree could be temporarily frozen).

Merging a change checks it into the repository.  Since changes are rebased onto
the `master` branch as they are merged, the commit hash in the repository may
differ from the one in your PR branch.

If your change has been approved for a few days without being merged, feel
free to write a comment in the PR requesting it.

## Miscellaneous topics

This section collects a number of other comments that are outside the
issue/edit/code review/submit process itself.

### AI-assisted development with OpenSpec

Contributors can optionally use [OpenSpec](https://github.com/Fission-AI/OpenSpec/)
for AI-assisted development. OpenSpec provides a structured workflow for creating
proposals, designs, specs, and implementation tasks with AI coding assistants like
Claude Code, GitHub Copilot, Gemini, and others.

**Setup:**

1. Install OpenSpec CLI (requires Node.js 20.19.0+):
   ```console
   $ npm install -g @fission-ai/openspec@latest
   ```

2. Run the setup script from the repository root:
   ```console
   $ ./_scripts/setup-openspec.sh
   ```

This creates local-only tooling files (gitignored) while specs and context are
tracked in `doc/`:

- `doc/specs/` - Main specs (source of truth, tracked)
- `doc/context/` - Shared context like language change checklists (tracked)
- `openspec/` - Workflow state and config (local only, gitignored)

**Quick start commands** (in your AI assistant):
- `/opsx:new` - Start a new change with proposal → design → specs → tasks workflow
- `/opsx:continue` - Continue working on an existing change
- `/opsx:apply` - Implement tasks from a change

**Updating after OpenSpec upgrades:**
```console
$ ./_scripts/setup-openspec.sh update
```

### Copyright headers

Files in the CUE repository don't list author names, both to avoid clutter and
to avoid having to keep the lists up to date.  Instead, your name will appear in
the [git change log](https://github.com/cue-lang/cue/commits/master)
and in [GitHub's contributor stats](https://github.com/cue-lang/cue/graphs/contributors)
when using an email address linked to a GitHub account.

New files that you contribute should use the standard copyright header
with the current year reflecting when they were added.
Do not update the copyright year for existing files that you change.

```
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
```

### Quickly testing your changes

Running `go test ./...` for every single change to the code tree is burdensome.
Even though it is strongly suggested to run it before sending a change, during
the normal development cycle you may want to compile and test only the package
you are developing.

In this section, we'll call the directory into which you cloned the CUE
repository `$CUEDIR`.  As CUE uses Go modules, The `cue` tool built by `go
install` will be installed in the `bin/go` in your home directory by default.

If you're changing the CUE APIs or code, you can test the results in just
this package directory.

```console
$ cd $CUEDIR/cue
$ [make changes...]
$ go test
```

You don't need to build a new cue tool to test it.
Instead you can run the tests from the root.

```console
$ cd $CUEDIR
$ go test ./...
```

To use the new tool you would still need to build and install it.

### Reviewing code by others

As part of the review process reviewers can propose changes directly, by
attaching commits to a pull request. You can check out someone else's proposed
changes locally by following the GitHub documentation on [checking out pull
requests locally](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/reviewing-changes-in-pull-requests/checking-out-pull-requests-locally),
or with the [`gh` CLI](https://cli.github.com/):

```console
$ gh pr checkout 9999
```

### Do I really have to add the `-s` flag to each commit?

Earlier in this guide we explained the role the [Developer Certificate of
Origin](https://developercertificate.org/) plays in contributions to the CUE
project. we also explained how `git commit -s` can be used to sign-off each
commit. But:

* it's easy to forget the `-s` flag;
* it's not always possible/easy to fix up other tools that wrap the `git commit` step.

You can automate the sign-off step using a [`git`
hook](https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks). Run the
following commands in the root of a `git` repository where you want to
automatically sign-off each commit:

```
cat <<'EOD' > .git/hooks/prepare-commit-msg
#!/bin/sh

NAME=$(git config user.name)
EMAIL=$(git config user.email)

if [ -z "$NAME" ]; then
    echo "empty git config user.name"
    exit 1
fi

if [ -z "$EMAIL" ]; then
    echo "empty git config user.email"
    exit 1
fi

git interpret-trailers --if-exists doNothing --trailer \
    "Signed-off-by: $NAME <$EMAIL>" \
    --in-place "$1"
EOD
chmod +x .git/hooks/prepare-commit-msg
```

If you already have a `prepare-commit-msg` hook, adapt it accordingly. The `-s`
flag will now be implied every time a commit is created.

## Code of Conduct

Guidelines for participating in CUE community spaces and a reporting process for
handling issues can be found in the [Code of Conduct](https://cuelang.org/docs/reference/code-of-conduct/).
