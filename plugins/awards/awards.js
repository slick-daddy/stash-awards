// Stash Awards plugin entry point.
//
// A vanilla IIFE loaded by Stash as a plain <script>; no build step. Wires up
// the navbar button on performer pages, registers the standalone awards page,
// and on that page resolves the Stash performer to IAFD and renders the awards
// listed on the performer's bio page.
//
// Data path:
//
//   /plugins/awards/:performerId
//     -> StashService.queryFindPerformer(id).name
//     -> IAFD search.asp?search=<name>
//     -> /person.rme/id=<uuid>/bio
//     -> #awards panel -> [{organization, year, result, awardName, ...}]
//
// Anything that fails along the way degrades gracefully to a notice + the IAFD
// deep-link; the plugin never throws at the user.
(function () {
  "use strict";

  const api = window.PluginApi;
  if (!api) {
    console.warn("stash-awards: window.PluginApi missing, plugin will not mount");
    return;
  }

  const { register, React, libraries, utils } = api;
  const { Bootstrap, ReactRouterDOM, FontAwesomeSolid } = libraries || {};
  const { Alert, Badge, Button, Card, Spinner } = Bootstrap || {};
  const { useHistory } = ReactRouterDOM || {};
  const { faArrowLeft, faSyncAlt } = FontAwesomeSolid || {};
  const { StashService } = utils || {};

  const IAFD_BASE = "https://www.iafd.com";
  const BROWSER_UA =
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36";

  // --- DOM helpers ---------------------------------------------------------

  function performerIdFromUrl() {
    const m = /^\/performers\/([^/?#]+)/.exec(window.location.pathname);
    return m ? decodeURIComponent(m[1]) : null;
  }

  function findNavbar() {
    const candidates = [
      document.querySelector(".performer-navbar"),
      document.querySelector(".performer-details .navbar"),
      document.querySelector(".navbar-buttons"),
      document.querySelector("[class*='performer'] [class*='navbar']"),
      document.querySelector("[class*='Performer'] [class*='Navbar']"),
    ];
    for (const c of candidates) {
      if (c && c.querySelector("button")) return c;
    }
    const allButtons = Array.from(document.querySelectorAll("button"));
    for (const b of allButtons) {
      const parent = b.closest("div, nav, header");
      if (!parent) continue;
      const rect = parent.getBoundingClientRect();
      if (rect.top > 200) continue;
      if (parent.querySelectorAll("button").length >= 2) return parent;
    }
    return null;
  }

  function navigate(path) {
    if (window.history && window.history.pushState) {
      window.history.pushState({}, "", path);
      window.dispatchEvent(new PopStateEvent("popstate"));
    } else {
      window.location.assign(path);
    }
  }

  function ensureAwardsButton() {
    const performerId = performerIdFromUrl();
    const existing = document.querySelector("button.awards-nav-button");

    if (!performerId) {
      if (existing) existing.remove();
      return;
    }

    const navbar = findNavbar();
    if (!navbar) return;

    let btn = existing;
    if (!btn) {
      btn = document.createElement("button");
      btn.type = "button";
      btn.className = "btn btn-secondary awards-nav-button";
      btn.textContent = "\uD83C\uDFC6 Awards";
      btn.addEventListener("click", () => navigate(`/plugins/awards/${performerId}`));
    }
    if (btn.parentElement !== navbar) {
      navbar.insertBefore(btn, navbar.firstChild);
    }
  }

  function startNavbarButton() {
    if (typeof MutationObserver === "undefined") return;
    if (!document.body) {
      document.addEventListener("DOMContentLoaded", startNavbarButton, { once: true });
      return;
    }
    const observer = new MutationObserver(() => {
      try {
        ensureAwardsButton();
      } catch (err) {
        console.warn("stash-awards: could not add the awards button", err);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("popstate", ensureAwardsButton);
    ensureAwardsButton();
  }

  // --- Stash + IAFD I/O ----------------------------------------------------

  // fetchPerformerName returns the performer's display name from Stash's
  // GraphQL. The StashService wrapper is not always present (depends on the
  // Stash version and loadableComponents state); any failure falls back to the
  // raw id so the user can still try the deep-link.
  async function fetchPerformerName(performerId) {
    if (!StashService || typeof StashService.queryFindPerformer !== "function") {
      return null;
    }
    try {
      const result = await StashService.queryFindPerformer(performerId);
      return (result && result.data && result.data.findPerformer && result.data.findPerformer.name) || null;
    } catch (err) {
      console.warn("stash-awards: could not read performer from Stash", err);
      return null;
    }
  }

  // iafdFetch wraps fetch with a browser User-Agent. IAFD returns 403 to
  // anything that does not look like a real browser, so this is non-optional.
  // Network/CORS errors are surfaced as thrown exceptions; HTTP 4xx/5xx are
  // returned as { ok, status } so callers can show a useful message.
  async function iafdFetch(url) {
    const resp = await fetch(url, {
      method: "GET",
      credentials: "omit",
      headers: {
        Accept: "text/html,application/xhtml+xml",
        "User-Agent": BROWSER_UA,
        "Accept-Language": "en-US,en;q=0.9",
      },
    });
    return resp;
  }

  function parseSearchResults(html) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    const seen = new Set();
    const performers = [];
    doc.querySelectorAll("tr").forEach((row) => {
      const links = row.querySelectorAll('a[href*="/person.rme/id="]');
      const a = links[links.length - 1];
      if (!a) return;
      const href = a.getAttribute("href") || "";
      const m = /\/person\.rme\/id=([^"'\s<>]+)/i.exec(href);
      if (!m) return;
      const id = m[1];
      if (seen.has(id)) return;
      seen.add(id);
      const name = (a.textContent || "").replace(/\s+/g, " ").trim();
      if (!name) return;
      performers.push({ id, name });
    });
    return performers;
  }

  // pickBestMatch selects the search hit whose name most closely matches the
  // query. Exact (case-insensitive) wins; otherwise the longest name that
  // starts with the query wins, then the shortest name overall (a unique
  // performer is more likely than one of many aliases).
  function pickBestMatch(query, candidates) {
    if (candidates.length === 0) return null;
    const q = query.trim().toLowerCase();
    const exact = candidates.find((c) => c.name.toLowerCase() === q);
    if (exact) return exact;
    const starts = candidates.filter((c) => c.name.toLowerCase().startsWith(q));
    if (starts.length > 0) {
      return starts.slice().sort((a, b) => b.name.length - a.name.length)[0];
    }
    return candidates.slice().sort((a, b) => a.name.length - b.name.length)[0];
  }

  // resolveIafdId returns { id, matchedName } for the given Stash performer
  // name, or null if no match was found / IAFD blocked the request.
  async function resolveIafdId(name) {
    if (!name) return null;
    const url = `${IAFD_BASE}/search.asp?search=${encodeURIComponent(name)}`;
    const resp = await iafdFetch(url);
    if (!resp.ok) {
      throw new Error(`IAFD search returned HTTP ${resp.status}`);
    }
    const html = await resp.text();
    const candidates = parseSearchResults(html);
    const best = pickBestMatch(name, candidates);
    return best ? { id: best.id, matchedName: best.name } : null;
  }

  // --- Awards parsing ------------------------------------------------------

  // normalize collapses whitespace and turns non-breaking spaces into the
  // spaces the regexes expect.
  function normalize(s) {
    return (s || "").replace(/\u00a0/g, " ").replace(/\s+/g, " ").trim();
  }

  // resolveUrl turns an IAFD-relative href into an absolute URL using the
  // performer's bio page as the base. Returns the original string if it is
  // already absolute or the base is unusable.
  function resolveUrl(href, pageUrl) {
    if (!href) return "";
    if (/^https?:\/\//i.test(href)) return href;
    try {
      return new URL(href, pageUrl).toString();
    } catch {
      return href.startsWith("/") ? href : "";
    }
  }

  // resultFromLabel maps the prefix IAFD puts in front of an entry onto a
  // result. "Winner"/"Won" -> won, "Nominee"/"Nominated" -> nominated,
  // "Inducted" -> inducted. Unrecognised labels fall back to the bold-means-
  // win rule the Go parser used.
  const labelToResult = {
    winner: "won",
    won: "won",
    nominee: "nominated",
    nominated: "nominated",
    inducted: "inducted",
  };

  function resultBadgeVariant(result) {
    if (result === "won") return "success";
    if (result === "inducted") return "warning";
    return "secondary";
  }

  // parseAwards reads a performer bio page HTML and returns every award in
  // its awards tab. The panel is a flat run of <p class="bioheading"> /
  // <div class="showyear"> / <div class="biodata"> siblings, so the
  // organization and year are carried forward while walking them.
  function parseAwards(html, pageUrl) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    const panel = doc.querySelector("#awards");
    if (!panel) return [];

    const out = [];
    let org = "";
    let year = 0;
    panel.querySelectorAll("p.bioheading, div.showyear, div.biodata").forEach((node) => {
      if (node.matches("p.bioheading")) {
        org = normalize(node.textContent);
        year = 0;
        return;
      }
      if (node.matches("div.showyear")) {
        const n = parseInt(normalize(node.textContent), 10);
        if (!Number.isNaN(n)) year = n;
        return;
      }
      // div.biodata
      if (!org || !year) return;
      const text = normalize(node.textContent);
      if (!text) return;
      const award = parseEntry(node, org, year, pageUrl);
      if (award) out.push(award);
    });
    return out;
  }

  // parseEntry reads one <div class="biodata"> into a single award record.
  // The shape follows the IAFD bio-page structure (a flat run of bioheading /
  // showyear / biodata siblings with the link as the split point between
  // the award name and the linked movie).
  function parseEntry(node, org, year, pageUrl) {
    const award = {
      organization: org,
      year: year,
      result: "nominated",
      awardName: "",
      category: undefined,
      associatedMovie: undefined,
      associatedMovieUrl: undefined,
      associatedMovieYear: undefined,
      event: `${org} ${year}`,
      sourceUrl: pageUrl,
    };

    const link = node.querySelector("a");
    let nameText;
    let tailText = "";
    if (link) {
      award.associatedMovie = normalize(link.textContent) || undefined;
      const href = link.getAttribute("href");
      award.associatedMovieUrl = resolveUrl(href, pageUrl) || undefined;
      nameText = normalize(textBefore(node, link));
      tailText = normalize(textAfter(node, link));
    } else {
      nameText = normalize(node.textContent);
    }

    nameText = nameText.replace(/[ ,;]+$/, "");
    if (!nameText) return null;

    const colonAt = nameText.indexOf(":");
    let label = "";
    let rest = nameText;
    if (colonAt >= 0) {
      label = nameText.slice(0, colonAt).trim().toLowerCase();
      rest = nameText.slice(colonAt + 1).trim();
    }

    const mapped = label ? labelToResult[label] : null;
    if (mapped) {
      award.result = mapped;
      award.awardName = rest;
    } else {
      award.awardName = nameText;
      if (node.querySelector("b, strong")) {
        award.result = "won";
      }
    }
    if (!award.awardName) return null;

    const movieYear = /\((\d{4})\)/.exec(tailText);
    if (movieYear) {
      award.associatedMovieYear = parseInt(movieYear[1], 10);
    }
    return award;
  }

  // walkText walks the descendants of parent in document order and returns
  // the concatenated text of everything before or after stop, depending
  // on after. Mirrors the IAFD parser's split-on-link trick: the link is
  // the only <a> in an entry, so everything before it is the award name
  // and everything after it (a year like "(2015)") is metadata about the
  // linked movie.
  function walkText(parent, stop, after) {
    const parts = [];
    let hit = false;
    const walker = document.createTreeWalker(parent, NodeFilter.SHOW_ALL);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      if (node === stop || stop.contains(node)) {
        hit = true;
        if (!after) return parts.join("");
        continue;
      }
      if (hit !== after) continue;
      if (node.nodeType === 3 /* text */) parts.push(node.nodeValue || "");
    }
    return parts.join("");
  }

  function textBefore(parent, stop) {
    return walkText(parent, stop, false);
  }

  function textAfter(parent, stop) {
    return walkText(parent, stop, true);
  }

  // groupByOrganization keeps first-appearance order, which matches the
  // page's own grouping since IAFD walks awards chronologically within each
  // organization.
  function groupByOrganization(awards) {
    const groups = [];
    const index = new Map();
    awards.forEach((a) => {
      let at = index.get(a.organization);
      if (at === undefined) {
        at = groups.length;
        index.set(a.organization, at);
        groups.push({ organization: a.organization, awards: [] });
      }
      groups[at].awards.push(a);
    });
    groups.forEach((g) => g.awards.sort((a, b) => b.year - a.year));
    return groups;
  }

  // --- Awards fetch pipeline ----------------------------------------------

  async function fetchAwardsForPerformer(performerId) {
    const name = await fetchPerformerName(performerId);
    if (!name) {
      throw new Error("Could not read the performer's name from Stash. Are you on the latest Stash?");
    }
    const match = await resolveIafdId(name);
    if (!match) {
      throw new Error(`No IAFD performer matched "${name}".`);
    }
    const bioUrl = `${IAFD_BASE}/person.rme/id=${match.id}/bio`;
    const resp = await iafdFetch(bioUrl);
    if (!resp.ok) {
      throw new Error(`IAFD returned HTTP ${resp.status} for the performer page.`);
    }
    const html = await resp.text();
    const awards = parseAwards(html, bioUrl);
    return {
      performerName: name,
      matchedName: match.matchedName,
      iafdUrl: bioUrl,
      awards,
    };
  }

  // --- React components ----------------------------------------------------

  function BackButton(props) {
    return React.createElement(
      Button,
      { variant: "secondary", size: "sm", onClick: props.onClick },
      React.createElement(FontAwesomeIcon, { icon: faArrowLeft }),
      " Back"
    );
  }

  function AwardRow({ award }) {
    const movieLink = award.associatedMovieUrl
      ? React.createElement(
          "a",
          { href: award.associatedMovieUrl, target: "_blank", rel: "noopener noreferrer" },
          award.associatedMovie
        )
      : award.associatedMovie || null;
    return React.createElement(
      "div",
      { className: "awards-row" },
      React.createElement("span", { className: "awards-year" }, String(award.year)),
      React.createElement(
        Badge,
        { variant: resultBadgeVariant(award.result) },
        award.result
      ),
      React.createElement(
        "span",
        { className: "awards-name" },
        award.awardName,
        award.associatedMovie
          ? React.createElement(
              React.Fragment,
              null,
              " \u2014 ",
              movieLink,
              award.associatedMovieYear ? ` (${award.associatedMovieYear})` : ""
            )
          : null
      )
    );
  }

  function AwardGroup({ organization, awards }) {
    return React.createElement(
      Card,
      { className: "awards-group" },
      React.createElement(Card.Header, null, organization),
      React.createElement(
        Card.Body,
        null,
        awards.map((a, i) => React.createElement(AwardRow, { key: `${a.year}-${i}`, award: a }))
      )
    );
  }

  function AwardsView({ data, onRetry }) {
    const groups = groupByOrganization(data.awards);
    return React.createElement(
      React.Fragment,
      null,
      React.createElement(
        "div",
        { className: "awards-source-actions" },
        React.createElement(
          "a",
          { href: data.iafdUrl, target: "_blank", rel: "noopener noreferrer" },
          "View on IAFD"
        ),
        React.createElement(
          Button,
          { variant: "secondary", size: "sm", onClick: onRetry },
          "Retry"
        )
      ),
      groups.length === 0
        ? React.createElement(
            Alert,
            { variant: "secondary" },
            "IAFD lists no awards for this performer."
          )
        : groups.map((g, i) =>
            React.createElement(AwardGroup, {
              key: `${g.organization}-${i}`,
              organization: g.organization,
              awards: g.awards,
            })
          )
    );
  }

  function AwardsPage(props) {
    const performerId =
      (props && props.match && props.match.params && props.match.params.performerId) || "";
    const history =
      typeof useHistory === "function" ? useHistory() : { push: (path) => navigate(path) };

    const [state, setState] = React.useState({ status: "loading" });
    const [reload, setReload] = React.useState(0);

    React.useEffect(() => {
      if (!performerId) return;
      let cancelled = false;
      setState({ status: "loading" });
      fetchAwardsForPerformer(performerId)
        .then((data) => {
          if (!cancelled) setState({ status: "ok", data });
        })
        .catch((err) => {
          if (!cancelled)
            setState({
              status: "error",
              message: err && err.message ? err.message : String(err),
            });
        });
      return () => {
        cancelled = true;
      };
    }, [performerId, reload]);

    if (!performerId) {
      return React.createElement(Alert, { variant: "danger" }, "No performer id in the page address.");
    }

    const title =
      (state.status === "ok" && (state.data.performerName || performerId)) || performerId;

    return React.createElement(
      "div",
      { className: "awards-page" },
      React.createElement(
        "div",
        { className: "awards-header" },
        React.createElement(BackButton, {
          onClick: () => history.push(`/performers/${performerId}`),
        }),
        React.createElement("h3", { className: "awards-title" }, title)
      ),
      state.status === "loading"
        ? React.createElement(Spinner, { animation: "border", role: "status" })
        : state.status === "error"
        ? React.createElement(
            Alert,
            { variant: "danger" },
            state.message,
            " ",
            React.createElement(
              Button,
              { size: "sm", onClick: () => setReload((n) => n + 1) },
              "Retry"
            )
          )
        : React.createElement(AwardsView, {
            data: state.data,
            onRetry: () => setReload((n) => n + 1),
          })
    );
  }

  // --- Mount ---------------------------------------------------------------

  register.route("/plugins/awards/:performerId", AwardsPage);
  startNavbarButton();
})();