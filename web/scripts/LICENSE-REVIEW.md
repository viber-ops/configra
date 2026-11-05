# UI license material maintenance

`npm run build` collects packages assigned to emitted JS chunks, including lazy
chunks and the reviewed Vite/Rolldown helper modules. The generated HTML links to
content-addressed notices and a UI-only CycloneDX 1.6 SBOM. The sign-in page and
workspace expose the notice link in both languages. These files are loaded only
when requested, not added to the main JS bundle.

`reviewed-licenses.json` binds the current 25 package versions to exact registry
archive integrity values and license-file hashes. All 26 listed files were
compared with their canonical npm archive entries on 2026-09-12. Their original
text is retained. Vite's full license appendix and Rolldown's Rollup/esbuild
attributions are retained as supersets for their emitted helpers; this does not
classify the entire native build-tool dependency tree as browser code.

On 2026-11-04 Vite moved from 8.2.2 to 8.3.5 (and Rolldown from 1.2.6 to
1.2.16) to clear high-severity Vite advisories. Rolldown's `LICENSE` and
`THIRD-PARTY-LICENSE` are byte-identical to the reviewed copies. Vite's core
MIT text is unchanged; its bundled-dependency appendix drops
`@vercel/detect-agent` (Apache-2.0), `cross-spawn`, `isexe`/`which`,
`path-key`/`shebang-regex` and `shebang-command`, adds `std-env` and `tinyexec`
(both MIT), and adds co-author names to two existing entries. No new license
type was introduced; the complete new appendix is retained as before.

Also on 2026-11-04, React and React DOM moved from 19.2.8 to 19.3.1 (Scheduler
from 0.27.0 to 0.28.0), `@codemirror/language` from 6.12.4 to 6.13.1,
`@codemirror/merge` from 6.12.2 to 6.12.3, `@codemirror/state` from 6.7.1 to
6.7.6, `@codemirror/view` from 6.43.9 to 6.43.15 and `@lezer/highlight` from
1.2.3 to 1.2.5. Each remains MIT, and its only legal file, `LICENSE`, is
byte-identical to the reviewed copy. Lockfile archive URLs and integrity values
match the npm registry. `@codemirror/language` now depends on
`@codemirror/streamparser` 6.0.0 (MIT, the same `LICENSE` bytes). Only
`StreamLanguage` reaches that package, and Configra does not use it, so no
module of it is assigned to an emitted chunk and it is not part of this
inventory. The build gate will require its review if that changes.

When upgrading a package or changing asset-generation paths, inspect the actual
build inputs and upstream licenses before updating this review. Unknown modules,
versions, generated helpers, or changed legal files must not be silently accepted.
Review CSS/static-asset loading changes as well; the current application has only
first-party CSS and no bundled third-party image/font asset pipeline.

```sh
# From web/; dependencies must have been installed with npm ci --ignore-scripts:
npm run test:licenses
npm test
```

The process tests exercise real production builds with an altered dependency
version, altered legal text, and stale output metadata. `export-licenses.mjs`
verifies the built asset hashes before copying the notice text and SBOM into
download bundles and the container filesystem. The same files remain embedded in
the server and available under their `/ui/assets/` URLs.

This is the UI portion of distribution verification, not server/runtime/CA
clearance or production-readiness approval.
