"use strict";
(() => {
  // src/plugin.ts
  var api = window.PluginApi;
  if (!api) {
    throw new Error(
      "stash-awards: window.PluginApi is missing; this script must be loaded by Stash"
    );
  }
  var React = api.React;
  var Apollo = api.Apollo;
  var StashService = api.utils.StashService;
  var register = api.register;
  var patch = api.patch;
  var Bootstrap = api.libraries.Bootstrap;
  var ReactRouterDOM = api.libraries.ReactRouterDOM;
  var FontAwesomeIcon = api.libraries.ReactFontAwesome.FontAwesomeIcon;
  var FontAwesomeSolid = api.libraries.FontAwesomeSolid;
  var useToast = api.hooks.useToast;

  // src/api.ts
  var PLUGIN_ID = "awards";
  var RUN_OPERATION = Apollo.gql`
  mutation RunPluginOperation($plugin_id: ID!, $args: Map) {
    runPluginOperation(plugin_id: $plugin_id, args: $args)
  }
`;
  async function run(args) {
    var _a;
    const client = StashService.getClient();
    const result = await client.mutate({
      mutation: RUN_OPERATION,
      variables: { plugin_id: PLUGIN_ID, args }
    });
    if ((_a = result.errors) == null ? void 0 : _a.length) {
      throw new Error(result.errors[0].message);
    }
    return result.data.runPluginOperation;
  }
  function getAwards(performerId, opts = {}) {
    var _a, _b;
    return run({
      mode: "getAwards",
      performerId,
      sync: (_a = opts.sync) != null ? _a : true,
      force: (_b = opts.force) != null ? _b : false
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
    return days === 1 ? "1 day ago" : `${days} days ago`;
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
    return groups;
  }
  function AwardRow({ award }) {
    var _a;
    const url = movieUrl(award);
    return /* @__PURE__ */ React.createElement("div", { className: "awards-row" }, /* @__PURE__ */ React.createElement("span", { className: "awards-year" }, award.year), /* @__PURE__ */ React.createElement(Badge, { variant: (_a = resultBadgeVariant[award.result]) != null ? _a : "secondary" }, award.result), /* @__PURE__ */ React.createElement("span", { className: "awards-name" }, award.awardName, award.category ? `: ${award.category}` : "", award.associatedMovie ? /* @__PURE__ */ React.createElement(React.Fragment, null, " \u2014 ", url ? /* @__PURE__ */ React.createElement("a", { href: url, target: "_blank", rel: "noopener noreferrer" }, award.associatedMovie) : award.associatedMovie, award.associatedMovieYear ? ` (${award.associatedMovieYear})` : "") : null));
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
    var _a;
    const awards = (_a = source.awards) != null ? _a : [];
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
    var _a, _b, _c, _d, _e, _f;
    const performerId = (_b = (_a = match == null ? void 0 : match.params) == null ? void 0 : _a.performerId) != null ? _b : "";
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
        var _a2;
        if (!cancelled) setLoadError((_a2 = err.message) != null ? _a2 : String(err));
      });
      return () => {
        cancelled = true;
      };
    }, [performerId, reload]);
    const refresh = async (source) => {
      var _a2;
      if (refreshing.has(source.source)) return;
      setRefreshing((prev) => new Set(prev).add(source.source));
      try {
        await syncSource(performerId, source.source);
        setPayload(await getAwards(performerId, { sync: false }));
        toast.success(`${source.label} refreshed`);
      } catch (err) {
        toast.error((_a2 = err.message) != null ? _a2 : String(err));
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
    const title = (_c = payload.performerName) != null ? _c : payload.performerId;
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
  var { Button: Button2 } = Bootstrap;
  function AwardsNavButton({ performerId }) {
    const history = ReactRouterDOM.useHistory();
    return /* @__PURE__ */ React.createElement(
      Button2,
      {
        variant: "secondary",
        title: "Awards",
        className: "awards-nav-button",
        onClick: () => history.push(`/plugins/awards/${performerId}`)
      },
      /* @__PURE__ */ React.createElement(FontAwesomeIcon, { icon: FontAwesomeSolid.faTrophy })
    );
  }
  function isAwardsNavbar(el) {
    var _a;
    const className = (_a = el == null ? void 0 : el.props) == null ? void 0 : _a.className;
    return typeof className === "string" && className.includes("details-edit") && className.includes("mb-2");
  }
  function inject(node, performerId) {
    var _a;
    if (Array.isArray(node)) return node.map((child) => inject(child, performerId));
    if (!React.isValidElement(node)) return node;
    const el = node;
    if (isAwardsNavbar(el)) {
      return React.cloneElement(
        el,
        {},
        ...React.Children.toArray(el.props.children),
        React.createElement(AwardsNavButton, { key: "awards-button", performerId })
      );
    }
    const children = (_a = el.props) == null ? void 0 : _a.children;
    if (children == null || typeof children === "function") return el;
    return React.cloneElement(el, {}, inject(children, performerId));
  }
  function injectAwardsButton(props, result) {
    var _a;
    try {
      const performerId = (_a = props == null ? void 0 : props.performer) == null ? void 0 : _a.id;
      if (!performerId || !result) return result;
      return inject(result, performerId);
    } catch (err) {
      console.warn("stash-awards: could not add the awards button", err);
      return result;
    }
  }

  // src/awards.ts
  register.route("/plugins/awards/:performerId", AwardsPage);
  patch.after("PerformerPage", injectAwardsButton);
})();
