# Changelog

## [3.15.0](https://github.com/musher-dev/mush/compare/v3.14.0...v3.15.0) (2026-03-14)


### Features

* **harness:** add experimental features framework, scrollback buffer, and collapsible sidebar ([#113](https://github.com/musher-dev/mush/issues/113)) ([fce9f6b](https://github.com/musher-dev/mush/commit/fce9f6bd1e8fdbbaa3424bb4ee5ab8321ceb1657))

## [3.14.0](https://github.com/musher-dev/mush/compare/v3.13.1...v3.14.0) (2026-03-12)


### Features

* **harness:** inject agents into project dir, add signal handling and exit watching ([#111](https://github.com/musher-dev/mush/issues/111)) ([d655660](https://github.com/musher-dev/mush/commit/d655660b96c0c6c83d6d28ba734c667864abb538))

## [3.13.1](https://github.com/musher-dev/mush/compare/v3.13.0...v3.13.1) (2026-03-11)


### Bug Fixes

* **bundle:** handle "other" asset type in bundle load session ([#109](https://github.com/musher-dev/mush/issues/109)) ([12087af](https://github.com/musher-dev/mush/commit/12087afd23b67dbd667973833bb6a2f88879d2bc))

## [3.13.0](https://github.com/musher-dev/mush/compare/v3.12.0...v3.13.0) (2026-03-11)


### Features

* **bundle:** add local directory loading, sample bundles, and My Bundles screen ([#107](https://github.com/musher-dev/mush/issues/107)) ([e33f3e4](https://github.com/musher-dev/mush/commit/e33f3e49aeebf97ffb607c56f3a04921eb662405))

## [3.12.0](https://github.com/musher-dev/mush/compare/v3.11.0...v3.12.0) (2026-03-09)


### Features

* **prompt:** show masked asterisks during password input ([#106](https://github.com/musher-dev/mush/issues/106)) ([17d2726](https://github.com/musher-dev/mush/commit/17d2726ebafeecb196de9dcbdc9c9a616497b7d0))


### Bug Fixes

* **hub:** guard against bundles with no published versions ([#96](https://github.com/musher-dev/mush/issues/96)) ([da5aa6c](https://github.com/musher-dev/mush/commit/da5aa6c51c091642d3f19e83e32a204369769f20))

## [3.11.0](https://github.com/musher-dev/mush/compare/v3.10.0...v3.11.0) (2026-03-07)


### Features

* **update:** add staged background auto-update agent ([#94](https://github.com/musher-dev/mush/issues/94)) ([6728a7e](https://github.com/musher-dev/mush/commit/6728a7e5b00b53d5aa65e311401b9d062120d699))

## [3.10.0](https://github.com/musher-dev/mush/compare/v3.9.0...v3.10.0) (2026-03-07)


### Features

* **bundle:** add contents panel, install tracking, and fix asset path doubling ([#91](https://github.com/musher-dev/mush/issues/91)) ([b13002d](https://github.com/musher-dev/mush/commit/b13002dad817a700b5b1f05a14371da5bbeb525d))

## [3.9.0](https://github.com/musher-dev/mush/compare/v3.8.0...v3.9.0) (2026-03-06)


### Features

* **harness:** modularize provider architecture and refine TUI bundle navigation ([#89](https://github.com/musher-dev/mush/issues/89)) ([88ab453](https://github.com/musher-dev/mush/commit/88ab45363523c9175083eb5e0d6167563a1f514a))

## [3.8.0](https://github.com/musher-dev/mush/compare/v3.7.0...v3.8.0) (2026-03-06)


### Features

* add multi-harness support, TUI navigation, and platform client enhancements ([#87](https://github.com/musher-dev/mush/issues/87)) ([c036bf0](https://github.com/musher-dev/mush/commit/c036bf0ee6ce122c9b07966248fb58a42642446b))
* **harness:** extract embedded runtime and refine status bar palette ([#88](https://github.com/musher-dev/mush/issues/88)) ([ac16637](https://github.com/musher-dev/mush/commit/ac166373c8badb692f3597d17bf3153b0a7e3fe6))
* **worker:** remove no-op `mush worker stop` and legacy harness compatibility shims
* **nav:** add hub exploration screen ([#85](https://github.com/musher-dev/mush/issues/85)) ([6f449f7](https://github.com/musher-dev/mush/commit/6f449f75c3c81919a48555fd5ade230a2e201785))

## [3.7.0](https://github.com/musher-dev/mush/compare/v3.6.0...v3.7.0) (2026-03-02)


### Features

* **harness:** rewrite CSI K erase-in-line and debounce sidebar redraw on CSI J ([#83](https://github.com/musher-dev/mush/issues/83)) ([b3ae7c4](https://github.com/musher-dev/mush/commit/b3ae7c41932cefff825a69185b9a5819995a6f5f))

## [3.6.0](https://github.com/musher-dev/mush/compare/v3.5.0...v3.6.0) (2026-03-02)


### Features

* **harness:** translate cursor-addressing sequences in sidebar output filter ([#78](https://github.com/musher-dev/mush/issues/78)) ([ac92911](https://github.com/musher-dev/mush/commit/ac92911507f1c96839c8fe86b81425b9f30f8051))

## [3.5.0](https://github.com/musher-dev/mush/compare/v3.4.0...v3.5.0) (2026-03-01)


### Features

* **harness:** add sidebar output filter, executor capability interfaces, and tamper quarantine ([#76](https://github.com/musher-dev/mush/issues/76)) ([0f80488](https://github.com/musher-dev/mush/commit/0f804888baed6ac18653b579c8384cc8fe1f754f))

## [3.4.0](https://github.com/musher-dev/mush/compare/v3.3.0...v3.4.0) (2026-03-01)


### Features

* **harness:** add cursor-rewrite filter to avoid SCOSC/DECSLRM conflict in sidebar mode ([#74](https://github.com/musher-dev/mush/issues/74)) ([68f8017](https://github.com/musher-dev/mush/commit/68f8017dfa5d6c2d4e422c3c2152067ae12e7748))

## [3.3.0](https://github.com/musher-dev/mush/compare/v3.2.0...v3.3.0) (2026-03-01)


### Features

* **harness:** decompose RootModel into TerminalController, JobLoop, and LR-margin modules ([#72](https://github.com/musher-dev/mush/issues/72)) ([7df67be](https://github.com/musher-dev/mush/commit/7df67be949a3f702ca51634c1d5d329ed7d2a6d3))

## [3.2.0](https://github.com/musher-dev/mush/compare/v3.1.0...v3.2.0) (2026-02-28)


### Features

* **harness:** improve scroll-region rendering, ANSI parsing, and terminal resilience ([#70](https://github.com/musher-dev/mush/issues/70)) ([0271b1d](https://github.com/musher-dev/mush/commit/0271b1d4579ec83245d8aba434312a7572de3eab))

## [3.1.0](https://github.com/musher-dev/mush/compare/v3.0.0...v3.1.0) (2026-02-28)


### Features

* **cli:** improve root help text with grouped commands and global --api-key flag ([#68](https://github.com/musher-dev/mush/issues/68)) ([1240a18](https://github.com/musher-dev/mush/commit/1240a18f48ea96233c71e0be8cba1829c2d7a49b))

## [3.0.0](https://github.com/musher-dev/mush/compare/v2.8.3...v3.0.0) (2026-02-26)


### ⚠ BREAKING CHANGES

* **worker:** `mush link` is removed; use `mush worker start` instead.

### Features

* **observability:** add opt-in OpenTelemetry tracing ([#66](https://github.com/musher-dev/mush/issues/66)) ([b5e5640](https://github.com/musher-dev/mush/commit/b5e5640da591b4d10a67281b24debfaf2b0c5578))
* **worker:** rename link to worker with improved UX and architecture enforcement ([#64](https://github.com/musher-dev/mush/issues/64)) ([86373ad](https://github.com/musher-dev/mush/commit/86373ad03586a60436c4dec3ef5b1144f8ec56b7))

## [2.8.3](https://github.com/musher-dev/mush/compare/v2.8.2...v2.8.3) (2026-02-21)


### Bug Fixes

* **bundle:** auto-repair SKILL.md YAML frontmatter with unquoted colons ([#59](https://github.com/musher-dev/mush/issues/59)) ([afa962d](https://github.com/musher-dev/mush/commit/afa962d64d51ebaf8a5cb65a8c863d855a422638))

## [2.8.2](https://github.com/musher-dev/mush/compare/v2.8.1...v2.8.2) (2026-02-21)


### Bug Fixes

* **bundle:** set workspace-write sandbox for codex bundle load ([#57](https://github.com/musher-dev/mush/issues/57)) ([fa4467e](https://github.com/musher-dev/mush/commit/fa4467e8a084fa6055f673e7de18681a0f50fb83))

## [2.8.1](https://github.com/musher-dev/mush/compare/v2.8.0...v2.8.1) (2026-02-21)


### Bug Fixes

* **bundle:** preserve CWD for codex bundle load and validate SKILL.md frontmatter ([#55](https://github.com/musher-dev/mush/issues/55)) ([e583791](https://github.com/musher-dev/mush/commit/e583791f67dc214ebaf25c62455bf89608653a1d))

## [2.8.0](https://github.com/musher-dev/mush/compare/v2.7.0...v2.8.0) (2026-02-21)


### Features

* **bundle:** extend asset injection to skills and use PTY for codex executor ([#53](https://github.com/musher-dev/mush/issues/53)) ([c638af6](https://github.com/musher-dev/mush/commit/c638af66288893946475f0569bf9e8006595acf6))

## [2.7.0](https://github.com/musher-dev/mush/compare/v2.6.1...v2.7.0) (2026-02-21)


### Features

* centralize path resolution, add agent injection and log rotation ([#51](https://github.com/musher-dev/mush/issues/51)) ([55c13f1](https://github.com/musher-dev/mush/commit/55c13f1e286818c26396a69c4e8eb9d7d9ddcca5))

## [2.6.1](https://github.com/musher-dev/mush/compare/v2.6.0...v2.6.1) (2026-02-21)


### Bug Fixes

* **harness:** move MCPProviderSpec to platform-independent file ([#48](https://github.com/musher-dev/mush/issues/48)) ([bb2c3fc](https://github.com/musher-dev/mush/commit/bb2c3fc55e7fedfdb2562041b7e7f53de3ea5686))

## [2.6.0](https://github.com/musher-dev/mush/compare/v2.5.0...v2.6.0) (2026-02-21)


### Features

* **harness:** add structured logging and data-driven provider specs ([#46](https://github.com/musher-dev/mush/issues/46)) ([390eec9](https://github.com/musher-dev/mush/commit/390eec969c6a2e11b5c6604b4a291a62221fec78))

## [2.5.0](https://github.com/musher-dev/mush/compare/v2.4.0...v2.5.0) (2026-02-20)


### Features

* **bundle:** support public bundle access without authentication ([#44](https://github.com/musher-dev/mush/issues/44)) ([31a7a00](https://github.com/musher-dev/mush/commit/31a7a00b27385394ca557b7ac6a039dd27151bc4))

## [2.4.0](https://github.com/musher-dev/mush/compare/v2.3.0...v2.4.0) (2026-02-20)


### Features

* **cli:** add argument validation, error handling, and golden tests ([#42](https://github.com/musher-dev/mush/issues/42)) ([18f1797](https://github.com/musher-dev/mush/commit/18f179745670205b45012d7edc2ee177a281e1a8))

## [2.3.0](https://github.com/musher-dev/mush/compare/v2.2.0...v2.3.0) (2026-02-19)


### Features

* **bundle:** adapt to workspace-scoped bundle API and runner asset endpoint ([#40](https://github.com/musher-dev/mush/issues/40)) ([5d69ae1](https://github.com/musher-dev/mush/commit/5d69ae118df2f270363fff44c05b915c8955fde1)), closes [#39](https://github.com/musher-dev/mush/issues/39)

## [2.2.0](https://github.com/musher-dev/mush/compare/v2.1.1...v2.2.0) (2026-02-19)


### Features

* **bundle:** rename run command to load ([#37](https://github.com/musher-dev/mush/issues/37)) ([d1ed973](https://github.com/musher-dev/mush/commit/d1ed973c7dd2b4fcf6653887e5d5af3e0bb56a36))

## [2.1.1](https://github.com/musher-dev/mush/compare/v2.1.0...v2.1.1) (2026-02-19)


### Bug Fixes

* **install:** replace hardcoded version with generic placeholder in help text ([#35](https://github.com/musher-dev/mush/issues/35)) ([1e19a7a](https://github.com/musher-dev/mush/commit/1e19a7a4549c6985d8f71e7e6ff6f12d58c7643b))

## [2.1.0](https://github.com/musher-dev/mush/compare/v2.0.0...v2.1.0) (2026-02-19)


### Features

* overhaul README with vanity install URL, badges, and full command reference ([#33](https://github.com/musher-dev/mush/issues/33)) ([a8965a8](https://github.com/musher-dev/mush/commit/a8965a8aad98473322954e96ad684a6ea7d62a74))

## [2.0.0](https://github.com/musher-dev/mush/compare/v1.5.0...v2.0.0) (2026-02-19)


### ⚠ BREAKING CHANGES

* **link:** mush link no longer supports --agent/-a; use --harness.

### Features

* **bundle:** add bundle commands with harness abstraction layer ([#32](https://github.com/musher-dev/mush/issues/32)) ([5a577b2](https://github.com/musher-dev/mush/commit/5a577b2ca808035221d3c04724e99bdae7d8b9b3))


### Miscellaneous

* **link:** rename runtime selector from agent to harness ([#28](https://github.com/musher-dev/mush/issues/28)) ([7e0d5b3](https://github.com/musher-dev/mush/commit/7e0d5b3905a7b27e1575666c194b38893b0327aa))

## [Unreleased]

### Breaking Changes

* **link:** replace `--agent` runtime selector with `--harness` (no backwards compatibility)

## [1.5.0](https://github.com/musher-dev/mush/compare/v1.4.0...v1.5.0) (2026-02-15)


### Features

* **harness:** improve PTY startup robustness and process group signal handling ([#26](https://github.com/musher-dev/mush/issues/26)) ([7de00e6](https://github.com/musher-dev/mush/commit/7de00e6f895c461a6ad44d5453ebb54ee5e11d4f))

## [1.4.0](https://github.com/musher-dev/mush/compare/v1.3.0...v1.4.0) (2026-02-15)


### Features

* **harness:** add graceful Ctrl+C shutdown and PTY process group management ([#24](https://github.com/musher-dev/mush/issues/24)) ([5cb0cf5](https://github.com/musher-dev/mush/commit/5cb0cf5cddd8fed9bf2bfb2c2030261f51962103))

## [1.3.0](https://github.com/musher-dev/mush/compare/v1.2.0...v1.3.0) (2026-02-15)


### Features

* **harness:** add live transcript streaming and improve harness ([#22](https://github.com/musher-dev/mush/issues/22)) ([85d0845](https://github.com/musher-dev/mush/commit/85d084524704b9fa0c5641da57482f8a2c758e01))

## [1.2.0](https://github.com/musher-dev/mush/compare/v1.1.0...v1.2.0) (2026-02-14)


### Features

* **harness:** add MCP server provisioning with credential rotation ([#19](https://github.com/musher-dev/mush/issues/19)) ([2b5413d](https://github.com/musher-dev/mush/commit/2b5413d1cc31ec27ef8587dbd49de3b7abd0e6ab))

## [1.1.0](https://github.com/musher-dev/mush/compare/v1.0.1...v1.1.0) (2026-02-10)


### Features

* **client:** consume GET /api/v1/runner/me for real SA identity ([#16](https://github.com/musher-dev/mush/issues/16)) ([bbaf3c1](https://github.com/musher-dev/mush/commit/bbaf3c10180ceda6d77bebff94ce140eb31407cc))

## [1.0.1](https://github.com/musher-dev/mush/compare/v1.0.0...v1.0.1) (2026-02-10)


### Documentation

* add community files and GitHub templates for open-source readiness ([#10](https://github.com/musher-dev/mush/issues/10)) ([55caa02](https://github.com/musher-dev/mush/commit/55caa02e8ab95f7700bc7b2e087810eb7ba684ef))

## 1.0.0 (2026-02-10)


### Features

* **cli:** add install script and shellcheck linting ([#8](https://github.com/musher-dev/mush/issues/8)) ([20e803a](https://github.com/musher-dev/mush/commit/20e803afa48173ba23fbb646ed7221159e9e3916))
* **cli:** add self-update system with background version checks ([#9](https://github.com/musher-dev/mush/issues/9)) ([7374dbf](https://github.com/musher-dev/mush/commit/7374dbfe5326b0d09478e301d5c37acc475c3b93))
* extract mush CLI from platform monorepo ([3f69848](https://github.com/musher-dev/mush/commit/3f6984889ae6fcf5b3db4b9cd16bedd303183183))


### Bug Fixes

* add .gitattributes to enforce LF line endings ([412251f](https://github.com/musher-dev/mush/commit/412251f69a1a3698f03798fd7bee5e2e8ed5b9bd))
* **cli:** allow dry-run without TTY and add --api-key flag to auth login ([#5](https://github.com/musher-dev/mush/issues/5)) ([746c87f](https://github.com/musher-dev/mush/commit/746c87fc9e7c56f63bb715802005232b9b91d8c7))
* **devcontainer:** use v2 module path for golangci-lint and fix gh config permissions ([#4](https://github.com/musher-dev/mush/issues/4)) ([75f405d](https://github.com/musher-dev/mush/commit/75f405de830b5565baa94046005e58ccde806ede))
* pin GitHub PR extension to v0.126.0 for VS Code compatibility ([0d635c8](https://github.com/musher-dev/mush/commit/0d635c8509309edae6afa486223e702941a82308))
