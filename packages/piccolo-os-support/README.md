# piccolo-os-support

Meta package that captures Piccolo OS specific base-system adjustments. Initially it
serves as a guardrail to ensure `piccolod` ships with every image. Future revisions
will add systemd units, transactional-update hooks, and health checks.

## Building
```
osc build openSUSE_Tumbleweed x86_64 piccolo-os-support.spec
```
Or with `rpmbuild`:
```
rpmbuild -ba piccolo-os-support.spec \
  --define '_sourcedir packages/piccolo-os-support' \
  --define '_specdir packages/piccolo-os-support' \
  --define '_srcrpmdir build' --define '_rpmdir build'
```

## Release flow
1. Update `Version`/`Release` inside the spec file when bumping requirements.
2. Tag the repo and upload the SRPM to OBS project `home:abhishekborar93:piccolo-os`.
3. Reference the built RPM inside the KIWI description (`packages type="image"`).
