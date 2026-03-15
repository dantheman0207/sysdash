# Homebrew Release Checklist

This is the standard release pattern used by `sysdash` and other `dantheman0207` Go CLI tools.

## Repo requirements

- Source repo contains:
  - `.goreleaser.yml`
  - `.github/workflows/release.yml`
  - a `--version` flag or equivalent
- GitHub Actions secret:
  - `HOMEBREW_TAP_GITHUB_TOKEN`

## Tap patterns

### Shared tools tap

Use this when the tool should install from:

```bash
brew tap dantheman0207/tools
brew install <tool>
```

GoReleaser `brews` entry:

- repository: `dantheman0207/homebrew-tools`
- directory: `Formula`

### Dedicated tap

Use this when backward compatibility or branding requires:

```bash
brew tap dantheman0207/<tool>
brew install <tool>
```

GoReleaser `brews` entry:

- repository: `dantheman0207/homebrew-<tool>`
- directory: `.`

### Dual tap

If both flows should work, publish the same formula to both taps.

Important:
- `brew install <tool>` is fine if the user taps only one of them.
- If both taps are installed, Homebrew may require a fully qualified formula name.

## Release steps

1. Make sure `main` is pushed.
2. Tag a release:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

3. GitHub Actions runs GoReleaser.
4. Verify:
   - GitHub Release contains the archive and `checksums.txt`
   - tap repo contains the updated formula
   - `brew install <tool>` works from the intended tap

## Local verification

```bash
brew install goreleaser
make release-snapshot
```

## Untap

```bash
brew untap dantheman0207/tools
brew untap dantheman0207/<tool>
```

