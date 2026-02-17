# Changelog

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
