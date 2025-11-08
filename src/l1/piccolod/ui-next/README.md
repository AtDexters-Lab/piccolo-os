# sv

Everything you need to build a Svelte project, powered by [`sv`](https://github.com/sveltejs/cli).

## Creating a project

If you're seeing this, you've probably already done this step. Congrats!

```sh
# create a new project in the current directory
npx sv create

# create a new project in my-app
npx sv create my-app
```

## Developing

Once you've created a project and installed dependencies with `npm install` (or `pnpm install` or `yarn`), start a development server:

```sh
npm run dev

# or start the server and open the app in a new browser tab
npm run dev -- --open
```

## Building

To create a production version of your app:

```sh
npm run build
```

You can preview the production build with `npm run preview`.

> To deploy your app, you may need to install an [adapter](https://svelte.dev/docs/kit/adapters) for your target environment.

## End-to-end portal logging test

We use Playwright to smoke-test the embedded portal and capture client-side diagnostics:

```sh
# defaults to http://piccolo.local; override with PICCOLO_BASE_URL
npm run e2e
```

Each test run attaches `console-log.json` and `network-log.json` artifacts so reviewers can inspect browser console output and the request waterfall. Set `PICCOLO_PORTAL_PATH` if you need to exercise a specific route, or run `npm run e2e:headed` to watch the flow locally.
