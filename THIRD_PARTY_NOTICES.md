# Third-party notices

Monitor Go's project-owned source and documentation use [Apache-2.0](LICENSE). Dependencies, fonts and icons retain their own licenses. [NOTICE](NOTICE) identifies the project attribution; it does not relicense upstream material.

The tables below inventory the direct dependencies pinned on 2026-10-14. Linked license/notice files are verbatim copies from the corresponding installed module or package, retained as `.txt` files. Preserve their text, including notices for bundled components. Do not translate or replace upstream legal terms.

## Go direct dependencies

| Module | Version | Purpose | License summary | Upstream license text |
|---|---|---|---|---|
| `github.com/boombuler/barcode` | `v1.1.0` | QR and barcode generation | MIT | [LICENSE](third_party/licenses/go/github.com_boombuler_barcode@v1.1.0/LICENSE.txt) |
| `github.com/go-sql-driver/mysql` | `v1.10.1` | MySQL target probing; not application storage | MPL-2.0 | [LICENSE](third_party/licenses/go/github.com_go-sql-driver_mysql@v1.10.1/LICENSE.txt) |
| `github.com/gofrs/flock` | `v0.13.1` | Single-instance data directory lock | BSD-3-Clause | [LICENSE](third_party/licenses/go/github.com_gofrs_flock@v0.13.1/LICENSE.txt) |
| `github.com/makiuchi-d/gozxing` | `v0.1.1` | Test-only decoding of generated codes | MIT | [LICENSE](third_party/licenses/go/github.com_makiuchi-d_gozxing@v0.1.1/LICENSE.txt) |
| `github.com/shirou/gopsutil/v4` | `v4.26.9` | Host metrics | BSD-3-Clause | [LICENSE](third_party/licenses/go/github.com_shirou_gopsutil_v4@v4.26.9/LICENSE.txt) |
| `golang.org/x/crypto` | `v0.58.0` | bcrypt password hashing | BSD-3-Clause | [LICENSE](third_party/licenses/go/golang.org_x_crypto@v0.58.0/LICENSE.txt) |
| `golang.org/x/image` | `v0.47.0` | Image processing, WebP and barcode text | BSD-3-Clause | [LICENSE](third_party/licenses/go/golang.org_x_image@v0.47.0/LICENSE.txt) |
| `modernc.org/sqlite` | `v1.60.1` | Embedded SQLite driver | BSD-3-Clause plus bundled notices | [LICENSE](third_party/licenses/go/modernc.org_sqlite@v1.60.1/LICENSE.txt) · [LICENSE-3RD-PARTY.md](third_party/licenses/go/modernc.org_sqlite@v1.60.1/LICENSE-3RD-PARTY.md.txt) · [LICENSE-SQLITE](third_party/licenses/go/modernc.org_sqlite@v1.60.1/LICENSE-SQLITE.txt) · [LICENSE-SQLITE_VEC](third_party/licenses/go/modernc.org_sqlite@v1.60.1/LICENSE-SQLITE_VEC.txt) |

## Frontend and tooling

| Package | Version | Purpose | Package license | Upstream license text |
|---|---|---|---|---|
| `@fontsource-variable/ibm-plex-sans` | `5.3.0` | Bundled font | OFL-1.1 | [LICENSE](third_party/licenses/npm/@fontsource-variable_ibm-plex-sans@5.3.0/LICENSE.txt) |
| `@fontsource/ibm-plex-mono` | `5.3.0` | Bundled font | OFL-1.1 | [LICENSE](third_party/licenses/npm/@fontsource_ibm-plex-mono@5.3.0/LICENSE.txt) |
| `lucide-react` | `1.57.0` | UI icons | ISC, with bundled MIT notices | [LICENSE](third_party/licenses/npm/lucide-react@1.57.0/LICENSE.txt) |
| `react` | `19.3.0` | UI runtime | MIT | [LICENSE](third_party/licenses/npm/react@19.3.0/LICENSE.txt) |
| `react-dom` | `19.3.0` | DOM renderer | MIT | [LICENSE](third_party/licenses/npm/react-dom@19.3.0/LICENSE.txt) |
| `zustand` | `5.0.15` | Typed application and view state | MIT | [LICENSE](third_party/licenses/npm/zustand@5.0.15/LICENSE.txt) |
| `@testing-library/react` | `16.3.3` | React interaction tests | MIT | [LICENSE](third_party/licenses/npm/@testing-library_react@16.3.3/LICENSE.txt) |
| `@types/react` | `19.3.0` | React type definitions | MIT | [LICENSE](third_party/licenses/npm/@types_react@19.3.0/LICENSE.txt) |
| `@types/react-dom` | `19.3.0` | DOM renderer type definitions | MIT | [LICENSE](third_party/licenses/npm/@types_react-dom@19.3.0/LICENSE.txt) |
| `@vitejs/plugin-react` | `6.1.2` | Development/build tooling | MIT | [LICENSE](third_party/licenses/npm/@vitejs_plugin-react@6.1.2/LICENSE.txt) |
| `jsdom` | `26.1.0` | DOM test environment | MIT | [LICENSE.txt](third_party/licenses/npm/jsdom@26.1.0/LICENSE.txt) |
| `prettier` | `3.6.2` | Development/build tooling | MIT | [LICENSE](third_party/licenses/npm/prettier@3.6.2/LICENSE.txt) |
| `typescript` | `5.9.3` | Development/build tooling | Apache-2.0 | [LICENSE.txt](third_party/licenses/npm/typescript@5.9.3/LICENSE.txt) · [ThirdPartyNoticeText.txt](third_party/licenses/npm/typescript@5.9.3/ThirdPartyNoticeText.txt) |
| `vite` | `8.3.4` | Development/build tooling | MIT | [LICENSE.md](third_party/licenses/npm/vite@8.3.4/LICENSE.md.txt) |
| `vitest` | `5.0.4` | Component test runner | MIT | [LICENSE.md](third_party/licenses/npm/vitest@5.0.4/LICENSE.md.txt) |

## Release inventory

The Docker build additionally uses the official `node:24.22.0-alpine3.24` frontend builder, `golang:1.26.9-alpine3.24` Go builder and `alpine:3.24.2` runtime, each pinned by multi-platform manifest digest in `Dockerfile`. Builder tools stay outside the final image. Alpine's `ca-certificates` provides trusted TLS roots and `tzdata` supplies time-zone data; the 2026-10-15 build resolved `ca-certificates 20260909-r0` and `tzdata 2026e-r0`. These OS packages come from the Alpine 3.24 repository, so record the resolved package inventory again for each deployment rebuild. The base distribution, BusyBox and installed OS packages retain their respective upstream licenses; they are not relicensed under Apache-2.0. Review the resolved base/OS package inventory for each rebuilt or distributed image as well as the application dependencies. No prebuilt registry image is published by this repository preparation.

These direct-dependency tables are not an exhaustive manifest of every transitive module linked into a specific release binary. `go.sum` and the npm lockfile record the dependency graph, not a complete license bundle. Before distributing binaries or bundled web assets:

1. Record the exact commit, target platform and toolchain; list dependencies with `go list -m all` and `npm --prefix web ls --all` after installing the locked versions.
2. Identify the Go modules and frontend assets actually included in each artifact, including transitive code, fonts, icons and bundled components.
3. Include the applicable license and attribution files alongside the project `LICENSE` and `NOTICE`. Retain source-availability and notice requirements where upstream licenses call for them.
4. Update this inventory and copied texts when a dependency changes. Verify the final archive, not only this source tree.

In particular, the MySQL driver carries MPL-2.0 terms, fonts carry OFL-1.1 terms, and the SQLite module includes additional bundled-component notices. The project's Apache-2.0 selection does not replace those terms. No claim of a completed binary-distribution license audit is made by this inventory.
