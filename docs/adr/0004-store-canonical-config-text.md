# Store canonical Config text

Every Config commit validates and formats its YAML or JSON before storing that formatted source as the authoritative Revision content. This gives stable validation and diffs while preserving comments and key ordering, at the accepted cost of not preserving the caller's exact whitespace and quoting.
