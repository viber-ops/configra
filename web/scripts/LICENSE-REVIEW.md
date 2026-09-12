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
