# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

- Two test packages, and they are not interchangeable: `go test .` runs the SDK unit
  tests in the root package (fake token/Graph endpoints, no credentials needed), while
  `go test ./tests` holds integration tests against a live mailbox that skip unless the
  OAuth environment variables in `tests/helpers_test.go` are set. Never commit a real
  token to `tests/`; read it from the environment.
- Token lifecycle and authentication error classification live in `client.go` and
  `auth_error.go`. Callers branch on the classified `*AuthError` rather than on error
  text; see the "Authentication errors" section of `README.md` before changing a kind,
  a sentinel, or the AADSTS code mapping, since smartcomex's email worker depends on
  telling "retry later" apart from "a human must rotate the client secret".

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
