# Contributing

Use a focused branch and include tests for behavior changes. Before opening a pull request, run:

```sh
npm ci
npm run check
npm test
npm run build
npm run lint:extension
npm run test:ui
(cd companion && go test -race ./... && go vet ./...)
```

Do not include project names, repository paths, credentials, browser-profile data, or generated companion configuration in commits, issues, test fixtures, or logs.

Only maintainers publish releases. Pull requests never receive Mozilla signing credentials.
