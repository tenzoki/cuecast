e2e test resolves testdata/ by hop-counting `..`, coupled to package nesting depth
---
`pkg/cuecast/engine/engine_e2e_test.go` locates the fixtures with `filepath.Join("..","..","..","testdata")`. The number of `..` segments is tied to how deep the `engine` package sits below the repo root. The `pkg/cuecast/` nesting refactor (260625) already had to bump this from 2 to 3 segments. Any further move silently breaks fixture resolution with no compile error — only a runtime test failure.
---
Directly relevant to the planned in-tree integration into unite-co-creator (`.../codebase/go/pkg/cuecast/`): if the e2e test + `testdata/` travel with the package (`cp -r pkg/cuecast`), the depth changes again and `../../../testdata` will point at the wrong place.

Robust fix (deferred — suite is currently green): anchor on the module root instead of hop-counting. Either walk upward from `runtime.Caller`'s file path until a `go.mod` is found, or add a small `findTestdata()` helper shared by the e2e test. Low priority; do as part of the integration step or when the package is next moved.

Filed by: coderev review of the pkg/cuecast/ restructure (260625-0707). Severity: low (latent; no current failure).
