# Stash Awards

A Stash plugin that lists performer award nominations, wins and hall-of-fame inductions from [IAFD](https://www.iafd.com) and [AdultIndustryAwards](https://adultindustryawards.com) on a dedicated page inside Stash.

This repository follows the [plugins-repo-template](https://github.com/stashapp/plugins-repo-template) layout. The plugin lives under [`plugins/awards/`](plugins/awards) and is published to a source index on GitHub Pages whenever [`plugins/**`](.github/workflows/deploy.yml) changes on `main`.

## Install

Add the source index URL:

```
https://slick-daddy.github.io/stash-awards/main/index.yml
```

to Stash's **Settings → Plugins → Available Plugins → Add Source**, then install **Stash Awards** from the list.

## Develop

Each plugin under [`plugins/`](plugins) is a self-contained directory. Its `*.yml` manifest, `*.js`, `*.css` and any other files are zipped verbatim by [`build_site.sh`](build_site.sh); no build step.

To run the deploy workflow against a fork:

1. **Settings → Pages** → set Build and deployment to **GitHub Actions**.
2. Push to `main` (or run the **Deploy repository to Github Pages** workflow manually).

## License

AGPL-3.0, matching Stash. See [LICENCE](LICENCE).