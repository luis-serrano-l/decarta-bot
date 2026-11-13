# How to cut a release

A release is a version tag. Pushing it makes the
[release workflow](../../.github/workflows/release.yml) publish:

- Linux binaries for amd64 and arm64, with `checksums.txt`, on the GitHub
  release;
- a container image for both architectures at
  `ghcr.io/luis-serrano-l/decarta-bot`, tagged `1.2.3`, `1.2` and `latest`.

The same tag makes CI publish the snap to the `candidate` channel.

## Steps

1. Choose the version by [Semantic Versioning](https://semver.org/): a new
   command or option is a minor version, a fix is a patch, and a change
   that breaks a menu address or the configuration is a major version.

2. In `CHANGELOG.md`, rename `Unreleased` to the version and today's date,
   add an empty `Unreleased` above it, and update the links at the bottom
   of the file. Merge that as `chore: release 1.2.3`.

   The workflow takes the release notes from this section and stops if it
   is missing.

3. Run the [smoke test](smoke-test.md) on `main`.

4. Tag the merge commit and push the tag:

   ```bash
   git switch main && git pull
   git tag -a v1.2.3 -m "v1.2.3"
   git push origin v1.2.3
   ```

5. Watch the run and check the result:

   ```bash
   gh run watch
   gh release view v1.2.3
   docker run --rm ghcr.io/luis-serrano-l/decarta-bot:1.2.3 --version
   ```

   The last command prints `decarta-bot 1.2.3`.

6. Promote the snap from `candidate` to `stable`, as described in
   [Run as a snap](run-as-snap.md#for-maintainers-publishing-the-snap).

## The first time

GitHub creates the container package as private. Make it public once, under
**Packages → decarta-bot → Package settings → Change visibility**.

## If the workflow fails

Nothing is published half-way that later runs cannot replace. Fix the
cause on `main`, then move the tag and push it again:

```bash
git tag -f -a v1.2.3 -m "v1.2.3"
git push -f origin v1.2.3
```

If the GitHub release was already created, delete it first with
`gh release delete v1.2.3`. Do this only for a release nobody has used yet;
otherwise publish a new patch version.
