# Stash Awards

A [Stash](https://github.com/stashapp/stash) plugin that collects performer award
nominations, wins and hall-of-fame inductions from external sources, stores them
locally, and shows them on a dedicated page inside Stash.

Award data lives on external sites — primarily
[IAFD](https://www.iafd.com) and [AdultIndustryAwards](https://adultindustryawards.com) —
and Stash has no built-in way to display it. This plugin fetches that data once,
caches it in its own SQLite database, and serves it back instantly on later
visits.

## Status

Under construction.

## License

AGPL-3.0, matching Stash itself. See [LICENSE](LICENSE).
