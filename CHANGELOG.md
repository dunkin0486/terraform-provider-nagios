# CHANGELOG

## [2.4.0](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.3.0...v2.4.0) (2026-07-31)


### Features

* add nagios-field skill for the add-a-field TDD workflow ([#140](https://github.com/dunkin0486/terraform-provider-nagios/issues/140)) ([68e3f81](https://github.com/dunkin0486/terraform-provider-nagios/commit/68e3f81781398dfcba67481db8570f9154fbc0d7)), closes [#135](https://github.com/dunkin0486/terraform-provider-nagios/issues/135)

## [2.3.0](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.2.0...v2.3.0) (2026-07-31)


### Features

* add group-assignment and nested-group-membership fields ([#133](https://github.com/dunkin0486/terraform-provider-nagios/issues/133)) ([1813e51](https://github.com/dunkin0486/terraform-provider-nagios/commit/1813e5131ef08064c186e9534b47328aa8a773bf))

## [2.2.0](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.1.3...v2.2.0) (2026-07-30)


### Features

* add nagios_timeperiod resource ([#86](https://github.com/dunkin0486/terraform-provider-nagios/issues/86) phase 2) ([#128](https://github.com/dunkin0486/terraform-provider-nagios/issues/128)) ([460c912](https://github.com/dunkin0486/terraform-provider-nagios/commit/460c912d042f00adce751ef828b088bf2018a071))

## [2.1.3](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.1.2...v2.1.3) (2026-07-30)


### Bug Fixes

* mark enabled RequiresReplace on nagios_authserver ([#120](https://github.com/dunkin0486/terraform-provider-nagios/issues/120)) ([0b03f24](https://github.com/dunkin0486/terraform-provider-nagios/commit/0b03f2472a7715ba8f0cb2a025d6f0f9b2856dff)), closes [#104](https://github.com/dunkin0486/terraform-provider-nagios/issues/104)

## [2.1.2](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.1.1...v2.1.2) (2026-07-29)


### Bug Fixes

* give gh a repo context in the release-please auto-merge step ([#115](https://github.com/dunkin0486/terraform-provider-nagios/issues/115)) ([ece3e3d](https://github.com/dunkin0486/terraform-provider-nagios/commit/ece3e3d3452a6402e5eb4029a87fc73e7f196a3c))

## [2.1.1](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.1.0...v2.1.1) (2026-07-29)


### Bug Fixes

* track examples/**/*.tf and terraform fmt them ([#112](https://github.com/dunkin0486/terraform-provider-nagios/issues/112)) ([e29b307](https://github.com/dunkin0486/terraform-provider-nagios/commit/e29b307052f704cfb04b5c181bcd5c08bee38739)), closes [#110](https://github.com/dunkin0486/terraform-provider-nagios/issues/110)

## [2.1.0](https://github.com/dunkin0486/terraform-provider-nagios/compare/v2.0.1...v2.1.0) (2026-07-29)


### Features

* add parents field to nagios_host ([#109](https://github.com/dunkin0486/terraform-provider-nagios/issues/109)) ([e193be4](https://github.com/dunkin0486/terraform-provider-nagios/commit/e193be475ec457296e0594a381a40cbc79f1f6d0))


### Bug Fixes

* run test workflow once per PR instead of twice ([#102](https://github.com/dunkin0486/terraform-provider-nagios/issues/102)) ([547e855](https://github.com/dunkin0486/terraform-provider-nagios/commit/547e8559f17bad0f88c3f961b0d748e31d1985a0)), closes [#101](https://github.com/dunkin0486/terraform-provider-nagios/issues/101)

## 2.0.0 (July 28, 2026)

BREAKING CHANGES:

* The provider is rewritten on `terraform-plugin-framework`, replacing the deprecated pre-2019 `github.com/hashicorp/terraform` SDK v1 monolith. Provider configuration and resource/data source schemas are unchanged in shape, but the provider address is now `registry.terraform.io/dunkin0486/nagios` ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Host's `2d_coords`/`3d_coords` attributes are renamed to `coords_2d`/`coords_3d` - Terraform attribute names can't start with a digit; this was previously silently tolerated by the old SDK but is rejected by the real `terraform` binary ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))

BUG FIXES:

* Fixes every `GetX` client function returning a non-nil, all-empty-fields struct on zero results instead of `nil` - this silently broke Terraform's not-found/state-clearing handling and caused an intermittent "provider produced inconsistent result after apply" failure ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes `authserver`'s Update handler being a complete no-op - `enabled` changes were never pushed to Nagios ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes `authserver`'s DELETE request sending the wrong field name in its body ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes `free_variables` on `host`/`service` never being read back correctly - Nagios returns these as dynamic top-level keys on the object, not nested under a `free_variables` key ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes optional boolean attributes silently sending Nagios an explicit `false` for unset values, indistinguishable from an intentional `false` ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes the live Nagios API token leaking in plaintext into Terraform diagnostics on any transient network failure ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Fixes malformed request URLs for any object name/description containing a space, for every mutating call except two that had a partial workaround ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))

IMPROVEMENTS:

* Adds the rename-after-manual-delete recreate fallback to `hostgroup`/`servicegroup`, which every other object type already had ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Docs are now generated via `tfplugindocs` from schema + `examples/`, replacing hand-written `docs/*.md` ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))
* Releases are now built and published via GoReleaser instead of a manual per-platform build loop ([#87](https://github.com/dunkin0486/terraform-provider-nagios/pull/87))

## 1.4.0 (December XX, 2019)

FEATURES:

* **New Data Source:** `data_source_host` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* **New Data Source:** `data_source_service` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* **New Data Source:** `data_source_hostgroup` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* **New Resource:** `resource_authserver` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))

IMPROVEMENTS:

* Changes resource `nagios_host` from using `name` to `host_name` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Removes all underscores from tests to comply with linting ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Renames `name` to `host_name` for resource `resource_host` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Several fixes to documentation formatting ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Adds `make resource` and `make data_source` for quicker creation of files ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))

BUG FIXES:

* Fixes formatting issue with `docs/resources/resource_host.md` ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Fixes inconsistencies within documentation ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))
* Removes `free_variables` attribute from `resource_contact` due to issue [#64](https://github.com/devopsdunkin/terraform-provider-nagios/issues/64) ([#65](https://github.com/devopsdunkin/terraform-provider-nagios/pull/65))

## 1.3.0 (December 16, 2019)

FEATURES:

* Adds automated GitHub releases through CircleCI pipeline ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))
* Adds `free_variables` field to `resource_host`, `resource_service` and `resource_contact` ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))

IMPROVEMENTS:

* Adds centralized function to create URL parameeters for all resources ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))
* Adds `omitempty` tag to all optional struct fields to prevent setting options when not specified in schema ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))
* Refactors `get` function in `client.go` to return `[]byte` to allow for more flexibility when performing an unmarshal of `[]byte` into an `interface{}` ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))
* Fixes formatting and linting issues with `docs/resources/resource_host.md` ([#59](https://github.com/devopsdunkin/terraform-provider-nagios/pull/59))

BUG FIXES:

* NA

## 1.2.0 (November 20, 2019)

FEATURES:

* **New Resource:** `resource_contact` ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* **New Resource:** `resource_contactgroup` ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))

IMPROVEMENTS:

* Adds link to GitHub releases page under the installing provider section of the documentation ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Adds support to import state for `resource_host`, `resource_service`, `resource_hostgroup`, `resource_servicegroup`, `resource_contact` and `resource_contactgroup` ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Removes unused `schema` tag on structs for `resource_host`, `resource_service`, `resource_hostgroup`, `resource_servicegroup` and `resource_contact` ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Adds `register` attribute for `resource_host` and `resource_service`. It is used to set whether the object is active or not in Nagios ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Removes unused files from .gitignore ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Updates README with updated roadmap and supported versions ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))

BUG FIXES:

* Fixes an issue where line breaks were missing from the documentation for resource arguments ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))
* Cleans up duplicate code ([#57](https://github.com/devopsdunkin/terraform-provider-nagios/pull/57))

## 1.1.1 (October 31, 2019)

FEATURES:

* None

IMPROVEMENTS:

* None

BUG FIXES:

* Fixes syntax issue with adding service description in when performing update to a service ([#52](https://github.com/devopsdunkin/terraform-provider-nagios/pull/52))
* Fixes syntax issue with replacing spaces with `%20` for attributes when performing an update ([#52](https://github.com/devopsdunkin/terraform-provider-nagios/pull/52))
* Fixes issue where service description was not getting passed as a URL parameter, so it would not update ([#52](https://github.com/devopsdunkin/terraform-provider-nagios/pull/52))

## 1.1.0 (October 30, 2019)

FEATURES:

* Adds CHANGELOG ([#51](https://github.com/devopsdunkin/terraform-provider-nagios/pull/51))
* Adds test job to pipeline ([#51](https://github.com/devopsdunkin/terraform-provider-nagios/pull/51))

IMPROVEMENTS:

* Cleans up unused code ([#51](https://github.com/devopsdunkin/terraform-provider-nagios/pull/51))

BUG FIXES:

* Fixed syntax errors in documentation ([#51](https://github.com/devopsdunkin/terraform-provider-nagios/pull/51))
* Adds PR link to changes in v1.0.0 ([#51](https://github.com/devopsdunkin/terraform-provider-nagios/pull/51))

## 1.0.0 (October 24, 2019)

FEATURES:

* **New Resource:** `resource_host` ([#43](https://github.com/devopsdunkin/terraform-provider-nagios/pull/43))
* **New Resource:** `resource_hostgroup` ([#43](https://github.com/devopsdunkin/terraform-provider-nagios/pull/43))
* **New Resource:** `resource_service` ([#43](https://github.com/devopsdunkin/terraform-provider-nagios/pull/43))
* **New Resource:** `resource_servicegroup` ([#43](https://github.com/devopsdunkin/terraform-provider-nagios/pull/43))
