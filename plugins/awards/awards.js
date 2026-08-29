// Stash Awards plugin entry point.
//
// A vanilla IIFE loaded by Stash as a plain <script>; no build step. Wires up
// the navbar button on performer pages and registers the standalone awards
// page. The page itself does not call any backend -- this plugin has none --
// and instead deep-links to the performer's profiles on IAFD and
// AdultIndustryAwards so the user can see the awards directly.
(function () {
  "use strict";

  const api = window.PluginApi;
  if (!api) {
    console.warn("stash-awards: window.PluginApi missing, plugin will not mount");
    return;
  }

  const { register, patch, React, libraries } = api;
  const { Bootstrap, ReactRouterDOM, FontAwesomeIcon, FontAwesomeSolid } = libraries || {};
  const { Alert, Button, Card, Spinner } = Bootstrap || {};
  const { useHistory } = ReactRouterDOM || {};
  const { faArrowLeft, faTrophy } = FontAwesomeSolid || {};

  // --- DOM helpers ---------------------------------------------------------

  // performPerformerId returns the :id from /performers/:id if the URL is a
  // performer page, else null.
  function performerIdFromUrl() {
    const m = /^\/performers\/([^/?#]+)/.exec(window.location.pathname);
    return m ? decodeURIComponent(m[1]) : null;
  }

  // findNavbar locates the performer page's action button row. Tries
  // well-known containers and falls back to the first row of buttons near the
  // top of the page.
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

  // ensureAwardsButton inserts the Awards button into the performer navbar
  // when on a performer page and removes it elsewhere.
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

  // startNavbarButton runs ensureAwardsButton on every DOM mutation so that
  // React re-renders do not erase the injected button.
  function startNavbarButton() {
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

  // --- Awards page ---------------------------------------------------------

  // sourceLinks maps source ids to the deep-link search URL the user can open
  // to see the performer's awards. The plugin does not scrape anything; it
  // shows a notice and these outbound links.
  const sourceLinks = [
    {
      id: "iafd",
      label: "IAFD",
      urlFor: (name) =>
        name
          ? `https://www.iafd.com/search.asp?search=${encodeURIComponent(name)}`
          : "https://www.iafd.com",
    },
    {
      id: "aia",
      label: "AdultIndustryAwards",
      urlFor: () => "https://adultindustryawards.com",
    },
  ];

  function BackButton(props) {
    return React.createElement(
      Button,
      { variant: "secondary", size: "sm", onClick: props.onClick },
      React.createElement(FontAwesomeIcon, { icon: faArrowLeft }),
      " Back"
    );
  }

  function AwardsPage(props) {
    const performerId = (props && props.match && props.match.params && props.match.params.performerId) || "";
    const history = (typeof useHistory === "function" ? useHistory() : null) || {
      push: (path) => navigate(path),
    };

    if (!performerId) {
      return React.createElement(Alert, { variant: "danger" }, "No performer id in the page address.");
    }

    return React.createElement(
      "div",
      { className: "awards-page" },
      React.createElement(
        "div",
        { className: "awards-header" },
        React.createElement(BackButton, {
          onClick: () => history.push(`/performers/${performerId}`),
        }),
        React.createElement("h3", { className: "awards-title" }, performerId)
      ),
      React.createElement(
        Alert,
        { variant: "secondary" },
        "Award scraping has been removed from this plugin. Use the source links below to look the performer up on the upstream sites."
      ),
      React.createElement(
        "div",
        { className: "awards-source-actions" },
        sourceLinks.map((src) =>
          React.createElement(
            "a",
            {
              key: src.id,
              href: src.urlFor(performerId),
              target: "_blank",
              rel: "noopener noreferrer",
            },
            `Look up on ${src.label}`
          )
        )
      )
    );
  }

  // --- Mount ---------------------------------------------------------------

  register.route("/plugins/awards/:performerId", AwardsPage);
  if (typeof patch === "object" && patch) {
    // patch.before/after/instead are intentionally unused; the navbar button
    // is injected by DOM so React re-renders do not delete it.
  }
  startNavbarButton();
})();