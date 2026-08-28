// The "Awards" entry button on the performer page.
//
// DetailsEditNavbar is not a named patch point, so the button is injected by
// patching after "PerformerPage" and walking the rendered element tree for the
// non-editing navbar (the one stash renders with classNames="mb-2"). If stash
// restructures that JSX the injection quietly does nothing — the awards page
// route itself keeps working, and the failure is only a console warning.
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

function isAwardsNavbar(el: any): boolean {
  const className = (el as any)?.props?.className;
  if (typeof className !== "string") return false;
  // v0.31.1 renders the performer details navbar as "details-edit mb-2".
  // Require details-edit to avoid false positives on unrelated mb-2
  // containers; mb-2 is checked secondarily for extra precision.
  const hasDetailsEdit = className.includes("details-edit");
  if (!hasDetailsEdit) return false;
  // Accept both strict and loose matches to survive minor class renames.
  return className.includes("mb-2") || hasDetailsEdit;
}

// inject returns a copy of the element tree with the awards button appended to
// the performer navbar. Anything it does not recognise is returned untouched.
function inject(node: any, performerId: string): any {
  if (Array.isArray(node)) return node.map((child) => inject(child, performerId));
  if (!React.isValidElement(node)) return node;

  const el = node as React.ReactElement<any>;

  if (isAwardsNavbar(el)) {
    const children = React.Children.toArray(el.props.children) as any[];
    // Idempotent: don't add twice on re-render.
    const already = children.some(
      (c) => c?.key === "awards-button" || c?.props?.className?.includes?.("awards-nav-button")
    );
    if (already) return el;
    return React.cloneElement(
      el,
      {},
      ...children,
      React.createElement(AwardsNavButton, { key: "awards-button", performerId })
    );
  }

  const children = el.props?.children;
  if (children == null || typeof children === "function") return el;
  return React.cloneElement(el, {}, inject(children, performerId));
}

// after("PerformerPage") hands this (props, renderedResult).
export function injectAwardsButton(props: any, result: any): any {
  try {
    const performerId = props?.performer?.id;
    if (!performerId || !result) return result;
    return inject(result, performerId);
  } catch (err) {
    console.warn("stash-awards: could not add the awards button", err);
    return result;
  }
}
