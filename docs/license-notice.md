# License And Notice Policy

BetterAltay-Go is distributed under the GNU Lesser General Public License v3.0, matching the BetterAltay source baseline used for the rewrite.

## Repository Files

- `LICENSE` contains the LGPL-3.0 license text for this project.
- `NOTICE` records the upstream projects used as behavioral and protocol references.
- Release artifacts must include both files.

## Attribution

- Keep BetterAltay, PocketMine-MP and Altay attribution in `NOTICE`, release notes and user-facing documentation.
- Do not add copyright headers to every new Go file by default.
- Add a short source note near a package or generated data file when it intentionally ports non-trivial upstream behavior, packet constants, tables or algorithms.

## Dependencies

- Third-party Go modules must keep their own license terms documented before release.
- Dependency license review is part of the release checklist, not a reason to scatter dependency notices through runtime code.

## Compatibility Boundary

The rewrite may use BetterAltay behavior, module boundaries and protocol constants as references, but it must not mechanically translate PHP files into Go. Any direct port of non-trivial upstream logic should be visible in documentation or local source notes.
