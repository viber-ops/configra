# Use mature Go libraries at system boundaries

Configra uses mature libraries directly for infrastructure concerns instead of building local frameworks: `go.yaml.in/yaml/v3` for YAML parsing, AST, and encoding; Go `encoding/json` for JSON parsing and formatting; Zap for structured application logs; and Cobra for the real `management` and `api` subcommands. Dependencies are added only when the calling code exists, and Configra does not wrap Zap or Cobra behind speculative interfaces.

No evaluated merge library matches Configra's complete directional overlay semantics: mappings recurse, Source replaces scalar and sequence values, Target-only keys remain, and Source `null` is a value rather than deletion. V1 therefore retains only a small tested traversal over the mature YAML/JSON representations; it does not implement a parser, formatter, or general-purpose merge engine. The evidence and migration thresholds are recorded in [Go library selection](../research/go-library-selection.md).
