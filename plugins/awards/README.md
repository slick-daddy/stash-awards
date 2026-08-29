# Stash Awards plugin

Adds an **Awards** tab to Stash performer pages that lists nominations, wins and hall-of-fame inductions pulled from [IAFD](https://www.iafd.com) and [AdultIndustryAwards](https://adultindustryawards.com).

This is the JS-only build of the plugin. It does not scrape the upstream sites itself — the Awards page deep-links out to the relevant search URLs so the user can see the performer's awards on the source site.

## Files

- `awards.yml` — Stash plugin manifest. The id Stash addresses the plugin by is the file name.
- `awards.js` — UI bundle. Vanilla IIFE, no build step.
- `awards.css` — layout for the Awards page and the navbar button.

## Develop

The plugin's source is the file Stash runs. To preview locally, link the directory into Stash's plugin path (`Settings → Plugins → Plugin Path`) and reload Stash.

To publish a new version, edit `version:` in `awards.yml` and push to `main`; `.github/workflows/deploy.yml` regenerates the source index on GitHub Pages.