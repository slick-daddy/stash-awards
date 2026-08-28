// The "Awards" entry button on the performer page.
//
// The button is injected by patching after "PerformerPage" and appending to the
// navbar. We deliberately avoid matching a specific Stash CSS class (it changes
// between versions and breaks silently) and instead look for the first action
// button in the rendered tree — the performer navbar's Edit button is the first
// in document order. The performer id is read from the props if present and,
// failing that, from the page URL, so the button appears regardless of how Stash
// happens to hand the performer to the patch.
import { React, Bootstrap, ReactRouterDOM, FontAwesomeIcon, FontAwesomeSolid } from "./plugin";

const { Button } = Bootstrap;

export function AwardsNavButton({ performerId }: { performerId: string }) {
  const history = ReactRouterDOM.useHistory();
  return (
    <Button
      variant="secondary"
      title="Awards"
      className="awards-nav-button"
      onClick={() => history.push(`/plugins/awards/${performerId}`)}
    >
      <FontAwesomeIcon icon={FontAwesomeSolid.faTrophy} />
    </Button>
  );
}

// getPerformerId tries every shape Stash has used for the performer page props,
// then falls back to the URL (/performers/:id), which is stable across versions.
function getPerformerId(props: any): string | undefined {
  const fromProps =
    props?.performer?.id ?? props?.match?.params?.id ?? props?.id;
  if (fromProps) return fromProps;
  const m = /\/performers?\/([^/?#]+)/.exec(window.location.pathname);
  return m?.[1];
}

// isButton catches the Bootstrap Button component by identity and a plain
// <button> element, so the navbar's Edit button is recognised on any Stash.
function isButton(el: any): boolean {
  if (!React.isValidElement(el)) return false;
  return el.type === Button || (typeof el.type === "string" && el.type === "button");
}

function alreadyAdded(children: any[]): boolean {
  return children.some(
    (c) => c?.props?.className?.includes?.("awards-nav-button")
  );
}

// inject walks the tree and, at the first container that holds an action button,
// appends the awards button next to it. Returns the tree untouched if it cannot
// find a home for the button.
function inject(node: any, performerId: string): any {
  if (Array.isArray(node)) return node.map((c) => inject(c, performerId));
  if (!React.isValidElement(node)) return node;

  const el = node as React.ReactElement<any>;
  const children = el.props?.children;
  if (children == null || typeof children === "function") return el;

  const childArr = React.Children.toArray(children);
  if (childArr.some(isButton)) {
    if (alreadyAdded(childArr)) return el;
    return React.cloneElement(
      el,
      {},
      ...childArr,
      React.createElement(AwardsNavButton, { key: "awards-button", performerId })
    );
  }

  return React.cloneElement(el, {}, inject(children, performerId));
}

// after("PerformerPage") hands this (props, renderedResult).
export function injectAwardsButton(props: any, result: any): any {
  try {
    const performerId = getPerformerId(props);
    if (!performerId || !result) return result;
    return inject(result, performerId);
  } catch (err) {
    console.warn("stash-awards: could not add the awards button", err);
    return result;
  }
}
