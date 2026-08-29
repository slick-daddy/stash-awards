# Stash Awards plugin

Adds an **Awards** tab to Stash performer pages that deep-links out to [IAFD](https://www.iafd.com) and [AdultIndustryAwards](https://adultindustryawards.com) so you can see the performer's awards on the source site.

This is a JS-only plugin with no server-side component. Cross-origin restrictions prevent live fetching of IAFD data from the browser, so the plugin provides direct search links instead.

## Files

| File | Purpose |
|------|---------|
| `awards.yml` | Plugin manifest consumed by Stash |
| `awards.js` | Vanilla JS entry point — no build step |
| `awards.css` | Minimal layout styles |

## How it works

1. On any performer page the plugin injects a **Awards** button into the navbar.
2. Clicking it navigates to `/plugins/awards/:performerId`.
3. That page shows a **Search on IAFD** link and a **Search on AdultIndustryAwards** link pre-filled with the performer's name.

## Installation

Add this source index in Stash under **Settings → Plugins → Available Plugins**:

```
https://slick-daddy.github.io/stash-awards/main/index.yml
```

Or copy `plugins/awards/` into your Stash `plugins/` directory and reload.
