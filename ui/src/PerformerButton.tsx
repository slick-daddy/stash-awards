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
  return (
    typeof className === "string" &&
    className.includes("details-edit") &&
    className.includes("mb-2")
  );
}

// inject returns a copy of the element tree with the awards button appended to
// the performer navbar. Anything it does not recognise is returned untouched.
function inject(node: any, performerId: string): any {
  if (Array.isArray(node)) return node.map((child) => inject(child, performerId));
  if (!React.isValidElement(node)) return node;

  const el = node as React.ReactElement<any>;

  if (isAwardsNavbar(el)) {
    return React.cloneElement(
      el,
      {},
      ...React.Children.toArray(el.props.children),
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
