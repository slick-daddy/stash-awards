// Stash Awards plugin entry point.
//
// A vanilla IIFE loaded by Stash as a plain <script>; no build step. Wires up
// the navbar button on performer pages and registers a standalone awards page
// that deep-links out to IAFD and AdultIndustryAwards search results for the
// current performer.
//
// This is a JS-only plugin with no server-side component. Cross-origin
// restrictions prevent live fetching of IAFD data from the browser, so the
// plugin provides direct links instead.
(function () {
  "use strict";

  const api = window.PluginApi;
  if (!api) {
    console.warn("stash-awards: window.PluginApi missing, plugin will not mount");
    return;
  }

  const { register, React, libraries } = api;
  const { Bootstrap, ReactRouterDOM, FontAwesomeSolid, ReactFontAwesome } = libraries || {};
  const { Button, Card } = Bootstrap || {};
  const { useHistory } = ReactRouterDOM || {};

  const IAFD_BASE = "https://www.iafd.com";
  const AIA_BASE = "https://adultindustryawards.com";

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

  // --- React components ----------------------------------------------------

  function AwardsPage(props) {
    const performerId =
      (props && props.match && props.match.params && props.match.params.performerId) || "";
    const history =
      typeof useHistory === "function" ? useHistory() : { push: (path) => navigate(path) };

    if (!performerId) {
      return React.createElement(
        "div",
        { className: "awards-page" },
        React.createElement("p", null, "No performer id in the page address.")
      );
    }

    const searchUrl = `${IAFD_BASE}/search.asp?search=${encodeURIComponent(performerId)}`;
    const aiaUrl = `${AIA_BASE}/search?q=${encodeURIComponent(performerId)}`;

    return React.createElement(
      "div",
      { className: "awards-page" },
      React.createElement(
        "div",
        { className: "awards-header" },
        React.createElement(
          Button,
          {
            variant: "secondary",
            size: "sm",
            onClick: () => history.push(`/performers/${performerId}`),
          },
          "\u2190 Back"
        ),
        React.createElement("h3", { className: "awards-title" }, "Awards")
      ),
      React.createElement(
        Card,
        { className: "awards-group" },
        React.createElement(Card.Header, null, "External Sources"),
        React.createElement(
          Card.Body,
          null,
          React.createElement(
            "p",
            null,
            "This plugin provides direct links to award databases. Click a link below to view this performer's awards on the source site."
          ),
          React.createElement(
            "div",
            { className: "awards-links" },
            React.createElement(
              "a",
              {
                href: searchUrl,
                target: "_blank",
                rel: "noopener noreferrer",
                className: "btn btn-outline-secondary btn-sm",
              },
              "Search on IAFD"
            ),
            React.createElement(
              "a",
              {
                href: aiaUrl,
                target: "_blank",
                rel: "noopener noreferrer",
                className: "btn btn-outline-secondary btn-sm",
              },
              "Search on AdultIndustryAwards"
            )
          )
        )
      )
    );
  }

  // --- Mount ---------------------------------------------------------------

  register.route("/plugins/awards/:performerId", AwardsPage);
  startNavbarButton();
})();
