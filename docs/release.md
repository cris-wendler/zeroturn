# Making a release

Making the repository public comes first, and has its own checklist in [going-public.md](going-public.md).

Nothing here publishes automatically. The workflow prepares a draft, and a person decides whether it becomes a release.

## Before tagging

1. `go test ./...`, `go vet ./...`, and `scripts/lint-copy.sh` pass on `main`.
2. `zeroturn doctor --compat --live` passes on a machine with the harness installed.
3. The README is current: rerun `docs/demo/record.sh` and the renderer if any command output changed.
4. `CHANGELOG.md` has an entry for the version, moved out of Unreleased.
5. The version is decided. The project follows semantic versioning from the first release. A change to an exit code, the event contract, or JSON output needs a major version.

## Building

```sh
scripts/build-release.sh 0.1.0
```

This writes `dist/`: one archive for macOS arm64 and amd64, Linux arm64 and amd64, and Windows amd64, each with the executable, the README, the changelog, and both license files, plus `SHA256SUMS`.

Check an archive before going further:

```sh
cd dist && shasum -a 256 -c SHA256SUMS
tar -xzf zeroturn_0.1.0_darwin_arm64.tar.gz && ./zeroturn_0.1.0_darwin_arm64/zeroturn version
```

## Draft release

Push the tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs the tests, builds the same archives, and opens a **draft** release with the archives and `SHA256SUMS` attached. Review the draft, then publish it by hand when you are ready.

## Homebrew

The formula lives in a separate tap repository, `cris-wendler/homebrew-tap`, as `Formula/zeroturn.rb`. Generate it after the archives exist:

```sh
scripts/homebrew-formula.sh 0.1.0 > zeroturn.rb
```

Test it locally before pushing it to the tap:

```sh
brew install --formula ./zeroturn.rb
zeroturn version
brew uninstall zeroturn
```

Installation then works with:

```sh
brew install cris-wendler/tap/zeroturn
```

## Go install

After the repository is public and the tag exists:

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
```

Test that command on a machine that has never built the project before adding it to the README.

## After publishing

1. Update the README installation section if the instructions changed.
2. Open an Unreleased section in `CHANGELOG.md` for the next version.
