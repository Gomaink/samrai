# samrai v0.2.0-rc.5

Release candidate 5 prepares the repository for public development and makes English the project language.

## English interface and server output

The browser interface, API error messages, command output, logs, scripts, test fixtures, and default instance labels are now in English. Existing titles, series names, notes, usernames, and other user-created data are left as they are.

A migration changes the managed library name from `Biblioteca principal` to `Main library` only when the old value is still the untouched default. Custom library names are not changed.

## Repository work

This release adds the files normally expected in a public repository:

- an MIT license;
- contribution and conduct guidelines;
- issue forms and a pull request template;
- Dependabot configuration;
- EditorConfig and Git attributes;
- a configuration reference;
- installation, development, release, and operational documentation in English.

The README now covers native Windows, native Unix, Docker, upgrades, backups, frontend development, and release checks from a clean checkout.

## Compatibility

The rename compatibility introduced in earlier candidates remains in place. Existing installations may continue to use:

- `PAGETURNER_*` environment variables;
- `pageturner.db` and `pageturner.log`;
- the legacy session cookie;
- the Docker volume `pageturner-data`.

New configuration should use the `SAMRAI_*` names.

## Upgrade from rc.4

1. Stop rc.4.
2. Extract rc.5 to a new directory.
3. Copy the complete rc.4 `data` directory.
4. Copy the rc.4 `.env` file.
5. Run the test script.
6. Start rc.5 and check login, one comic, one PDF or EPUB, and the import screen.

No imported file or user content needs to be renamed.

## Still known

- This is a release candidate, not the final `0.2.0` release.
- Restores still require the server to be stopped.
- Native CBR and CB7 imports require 7-Zip or `lsar` + `unar`.
- DRM-protected EPUB files are not supported.
