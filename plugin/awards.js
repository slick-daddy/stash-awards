/* GENERATED -- do not edit. Source: ui/src/*, built via npm run build in ui/ */
"use strict";
(() => {
  // src/plugin.ts
  var api = window.PluginApi;
  if (!api) {
    console.warn(
      "stash-awards: window.PluginApi is missing; UI will not mount outside Stash"
    );
  }
  var noop = () => {
  };
  var noopComponent = () => null;
  var _a;
  var React = (_a = api == null ? void 0 : api.React) != null ? _a : {
    createElement: noopComponent,
    isValidElement: () => false,
    Children: { toArray: (x) => Array.isArray(x) ? x : x ? [x] : [] },
    useState: () => [null, noop],
    useEffect: noop,
    Fragment: "fragment"
  };
  var _a2;
  var Apollo = (_a2 = api == null ? void 0 : api.Apollo) != null ? _a2 : { gql: (x) => x };
  var _a3, _b;
  var StashService = (_b = (_a3 = api == null ? void 0 : api.utils) == null ? void 0 : _a3.StashService) != null ? _b : {
    getClient: () => ({ mutate: async () => ({ data: {} }) })
  };
  var _a4;
  var register = (_a4 = api == null ? void 0 : api.register) != null ? _a4 : { route: noop };
  var _a5;
  var patch = (_a5 = api == null ? void 0 : api.patch) != null ? _a5 : { before: noop, after: noop, instead: noop };
  var _a6, _b2;
  var Bootstrap = (_b2 = (_a6 = api == null ? void 0 : api.libraries) == null ? void 0 : _a6.Bootstrap) != null ? _b2 : {};
  var _a7, _b3;
  var ReactRouterDOM = (_b3 = (_a7 = api == null ? void 0 : api.libraries) == null ? void 0 : _a7.ReactRouterDOM) != null ? _b3 : {
    useHistory: () => ({ push: noop })
  };
  var _a8, _b4, _c;
  var FontAwesomeIcon = (_c = (_b4 = (_a8 = api == null ? void 0 : api.libraries) == null ? void 0 : _a8.ReactFontAwesome) == null ? void 0 : _b4.FontAwesomeIcon) != null ? _c : noopComponent;
  var _a9, _b5;
  var FontAwesomeSolid = (_b5 = (_a9 = api == null ? void 0 : api.libraries) == null ? void 0 : _a9.FontAwesomeSolid) != null ? _b5 : {};
  var _a10, _b6;
  var useToast = (_b6 = (_a10 = api == null ? void 0 : api.hooks) == null ? void 0 : _a10.useToast) != null ? _b6 : (() => ({ success: noop, error: noop }));

  // src/api.ts
  var PLUGIN_ID = "awards";
  var RUN_OPERATION = Apollo.gql`
  mutation RunPluginOperation($plugin_id: ID!, $args: Map) {
    runPluginOperation(plugin_id: $plugin_id, args: $args)
  }
`;
  async function run(args) {
    var _a11;
    const client = StashService.getClient();
    const result = await client.mutate({
      mutation: RUN_OPERATION,
      variables: { plugin_id: PLUGIN_ID, args }
    });
    if ((_a11 = result.errors) == null ? void 0 : _a11.length) {
      throw new Error(result.errors[0].message);
    }
    return result.data.runPluginOperation;
  }
  function getAwards(performerId, opts = {}) {
    var _a11, _b7;
    return run({
      mode: "getAwards",
      performerId,
      sync: (_a11 = opts.sync) != null ? _a11 : true,
      force: (_b7 = opts.force) != null ? _b7 : false
    });
  }
  function syncSource(performerId, source) {
    return run({
      mode: "sync",
      performerId,
      source,
      force: true
    });
  }

  // src/AwardsPage.tsx
  var { Alert, Badge, Button, Card, Spinner, Tabs, Tab } = Bootstrap;
  function timeAgo(iso) {
    const then = Date.parse(iso);
    if (Number.isNaN(then)) return iso;
    const seconds = Math.max(0, Math.floor((Date.now() - then) / 1e3));
    if (seconds < 60) return "just now";
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return minutes === 1 ? "1 minute ago" : `${minutes} minutes ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return hours === 1 ? "1 hour ago" : `${hours} hours ago`;
    const days = Math.floor(hours / 24);
    if (days < 7) return days === 1 ? "1 day ago" : `${days} days ago`;
    const weeks = Math.floor(days / 7);
    if (weeks < 5) return weeks === 1 ? "1 week ago" : `${weeks} weeks ago`;
    const months = Math.floor(days / 30);
    if (months < 12) return months === 1 ? "1 month ago" : `${months} months ago`;
    const years = Math.floor(days / 365);
    return years === 1 ? "1 year ago" : `${years} years ago`;
  }
  function movieUrl(award) {
    const url = award.associatedMovieUrl;
    if (!url) return null;
    if (/^https?:\/\//i.test(url)) return url;
    if (award.sourceUrl) {
      try {
        return new URL(url, award.sourceUrl).toString();
      } catch {
      }
      return null;
    }
    return url.startsWith("/") ? url : null;
  }
  var resultBadgeVariant = {
    won: "success",
    nominated: "secondary",
    inducted: "warning"
  };
  function groupByOrganization(awards) {
    const groups = [];
    const index = /* @__PURE__ */ new Map();
    for (const award of awards) {
      let at = index.get(award.organization);
      if (at === void 0) {
        at = groups.length;
        index.set(award.organization, at);
        groups.push({ organization: award.organization, awards: [] });
      }
      groups[at].awards.push(award);
    }
    for (const g of groups) {
      g.awards.sort((a, b) => b.year - a.year);
    }
    return groups;
  }
  function AwardRow({ award }) {
    var _a11;
    const url = movieUrl(award);
    return /* @__PURE__ */ React.createElement("div", { className: "awards-row" }, /* @__PURE__ */ React.createElement("span", { className: "awards-year" }, award.year), /* @__PURE__ */ React.createElement(Badge, { variant: (_a11 = resultBadgeVariant[award.result]) != null ? _a11 : "secondary" }, award.result), /* @__PURE__ */ React.createElement("span", { className: "awards-name" }, award.awardName, award.category ? `: ${award.category}` : "", award.associatedMovie ? /* @__PURE__ */ React.createElement(React.Fragment, null, " \u2014 ", url ? /* @__PURE__ */ React.createElement("a", { href: url, target: "_blank", rel: "noopener noreferrer" }, award.associatedMovie) : award.associatedMovie, award.associatedMovieYear ? ` (${award.associatedMovieYear})` : "") : null));
  }
  function AwardGroup({
    organization,
    awards
  }) {
    return /* @__PURE__ */ React.createElement(Card, { className: "awards-group" }, /* @__PURE__ */ React.createElement(Card.Header, null, organization), /* @__PURE__ */ React.createElement(Card.Body, null, awards.map((award) => /* @__PURE__ */ React.createElement(AwardRow, { key: award.id, award }))));
  }
  function SourcePanel({
    source,
    onRefresh,
    refreshing
  }) {
    var _a11;
    const awards = (_a11 = source.awards) != null ? _a11 : [];
    if (!source.enabled) {
      return /* @__PURE__ */ React.createElement(Alert, { variant: "secondary" }, source.label, " is turned off in the plugin settings, so its awards are not shown.");
    }
    return /* @__PURE__ */ React.createElement("div", { className: "awards-source" }, /* @__PURE__ */ React.createElement("div", { className: "awards-source-actions" }, /* @__PURE__ */ React.createElement(
      Button,
      {
        variant: "secondary",
        size: "sm",
        disabled: refreshing,
        onClick: () => onRefresh(source)
      },
      /* @__PURE__ */ React.createElement(
        FontAwesomeIcon,
        {
          icon: FontAwesomeSolid.faSyncAlt,
          spin: refreshing
        }
      ),
      refreshing ? " Refreshing\u2026" : " Refresh"
    ), source.url ? /* @__PURE__ */ React.createElement("a", { href: source.url, target: "_blank", rel: "noopener noreferrer" }, "view on ", source.label) : null), source.error ? /* @__PURE__ */ React.createElement(Alert, { variant: "warning" }, "Last refresh failed: ", source.error) : null, awards.length === 0 ? /* @__PURE__ */ React.createElement(Alert, { variant: "secondary" }, "No awards found from ", source.label, ".") : groupByOrganization(awards).map((group) => /* @__PURE__ */ React.createElement(
      AwardGroup,
      {
        key: group.organization,
        organization: group.organization,
        awards: group.awards
      }
    )), source.lastSynced ? /* @__PURE__ */ React.createElement("p", { className: "awards-last-updated" }, "Last updated: ", timeAgo(source.lastSynced)) : null);
  }
  function BackButton({ onClick }) {
    return /* @__PURE__ */ React.createElement(Button, { variant: "secondary", size: "sm", onClick }, /* @__PURE__ */ React.createElement(FontAwesomeIcon, { icon: FontAwesomeSolid.faArrowLeft }), " Back");
  }
  var AwardsPage = ({ match }) => {
    var _a11, _b7, _c2, _d, _e, _f;
    const performerId = (_b7 = (_a11 = match == null ? void 0 : match.params) == null ? void 0 : _a11.performerId) != null ? _b7 : "";
    const history = ReactRouterDOM.useHistory();
    const toast = useToast();
    const [payload, setPayload] = React.useState(null);
    const [loadError, setLoadError] = React.useState(null);
    const [refreshing, setRefreshing] = React.useState(/* @__PURE__ */ new Set());
    const [reload, setReload] = React.useState(0);
    React.useEffect(() => {
      if (!performerId) return;
      let cancelled = false;
      setPayload(null);
      setLoadError(null);
      getAwards(performerId).then((data) => {
        if (!cancelled) setPayload(data);
      }).catch((err) => {
        var _a12;
        if (!cancelled) setLoadError((_a12 = err.message) != null ? _a12 : String(err));
      });
      return () => {
        cancelled = true;
      };
    }, [performerId, reload]);
    const refresh = async (source) => {
      var _a12;
      if (refreshing.has(source.source)) return;
      setRefreshing((prev) => new Set(prev).add(source.source));
      try {
        await syncSource(performerId, source.source);
        setPayload(await getAwards(performerId, { sync: false }));
        toast.success(`${source.label} refreshed`);
      } catch (err) {
        toast.error((_a12 = err.message) != null ? _a12 : String(err));
      } finally {
        setRefreshing((prev) => {
          const next = new Set(prev);
          next.delete(source.source);
          return next;
        });
      }
    };
    if (!performerId) {
      return /* @__PURE__ */ React.createElement(Alert, { variant: "danger" }, "No performer id in the page address.");
    }
    if (loadError) {
      return /* @__PURE__ */ React.createElement("div", { className: "awards-page" }, /* @__PURE__ */ React.createElement(BackButton, { onClick: () => history.push(`/performers/${performerId}`) }), /* @__PURE__ */ React.createElement(Alert, { variant: "danger" }, "Could not load awards: ", loadError, /* @__PURE__ */ React.createElement("div", null, /* @__PURE__ */ React.createElement(Button, { size: "sm", onClick: () => setReload(reload + 1) }, "Retry"))));
    }
    if (!payload) {
      return /* @__PURE__ */ React.createElement("div", { className: "awards-page" }, /* @__PURE__ */ React.createElement(BackButton, { onClick: () => history.push(`/performers/${performerId}`) }), /* @__PURE__ */ React.createElement(Spinner, { animation: "border", role: "status" }));
    }
    const title = (_c2 = payload.performerName) != null ? _c2 : payload.performerId;
    const defaultSource = (_f = (_d = payload.sources.find((s) => s.enabled)) == null ? void 0 : _d.source) != null ? _f : (_e = payload.sources[0]) == null ? void 0 : _e.source;
    return /* @__PURE__ */ React.createElement("div", { className: "awards-page" }, /* @__PURE__ */ React.createElement("div", { className: "awards-header" }, /* @__PURE__ */ React.createElement(BackButton, { onClick: () => history.push(`/performers/${performerId}`) }), /* @__PURE__ */ React.createElement("h3", { className: "awards-title" }, title), /* @__PURE__ */ React.createElement("span", { className: "awards-total" }, payload.total, " award", payload.total === 1 ? "" : "s")), payload.warning ? /* @__PURE__ */ React.createElement(Alert, { variant: "warning" }, payload.warning) : null, /* @__PURE__ */ React.createElement(Tabs, { defaultActiveKey: defaultSource, id: "awards-sources" }, payload.sources.map((source) => /* @__PURE__ */ React.createElement(
      Tab,
      {
        key: source.source,
        eventKey: source.source,
        title: `${source.label}${source.enabled ? ` (${source.count})` : ""}`,
        mountOnEnter: true,
        unmountOnExit: false
      },
      /* @__PURE__ */ React.createElement(
        SourcePanel,
        {
          source,
          onRefresh: refresh,
          refreshing: refreshing.has(source.source)
        }
      )
    ))));
  };

  // src/PerformerButton.tsx
  var NAV_BUTTON_CLASS = "awards-nav-button";
  function findNavbar() {
    const candidates = [
      document.querySelector(".performer-navbar"),
      document.querySelector(".performer-details .navbar"),
      document.querySelector(".navbar-buttons"),
      document.querySelector("[class*='performer'] [class*='navbar']"),
      document.querySelector("[class*='Performer'] [class*='Navbar']")
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
  function buildButton(performerId) {
    const existing = document.querySelector(
      `button.${NAV_BUTTON_CLASS}`
    );
    if (existing) return existing;
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `btn btn-secondary ${NAV_BUTTON_CLASS}`;
    btn.title = "Awards";
    btn.style.marginRight = "8px";
    btn.textContent = "\u{1F3C6} Awards";
    btn.addEventListener("click", () => navigate(`/plugins/awards/${performerId}`));
    return btn;
  }
  function performerIdFromUrl() {
    const m = /^\/performers\/([^/?#]+)/.exec(window.location.pathname);
    return m ? decodeURIComponent(m[1]) : null;
  }
  function ensureButton() {
    const performerId = performerIdFromUrl();
    const existing = document.querySelector(
      `button.${NAV_BUTTON_CLASS}`
    );
    if (!performerId) {
      if (existing) existing.remove();
      return;
    }
    const navbar = findNavbar();
    if (!navbar) {
      return;
    }
    const btn = buildButton(performerId);
    if (btn.parentElement === navbar) return;
    navbar.insertBefore(btn, navbar.firstChild);
  }
  function startAwardsButton() {
    if (!document.body) {
      document.addEventListener("DOMContentLoaded", startAwardsButton, {
        once: true
      });
      return;
    }
    const observer = new MutationObserver(() => {
      try {
        ensureButton();
      } catch (err) {
        console.warn("stash-awards: could not add the awards button", err);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("popstate", ensureButton);
    ensureButton();
  }

  // src/awards.ts
  register.route("/plugins/awards/:performerId", AwardsPage);
  startAwardsButton();
})();
