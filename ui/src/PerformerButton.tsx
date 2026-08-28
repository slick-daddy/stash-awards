// The "Awards" entry button on the performer page.
//
// The button is injected by patching after "PerformerPage" and prepending a
// simple button to the page. We deliberately do NOT walk and re-clone Stash's
// own element tree (that silently corrupted Stash's navbar components in some
// versions); prepending a fragment is safe and version-agnostic. The performer
// id is read from the props if present and, failing that, from the page URL
// (/performers/:id), so the button appears regardless of how Stash hands the
// performer to the patch.
import { React, Bootstrap, ReactRouterDOM } from "./plugin";

const { Button } = Bootstrap;

export function AwardsNavButton({ performerId }: { performerId: string }) {
  // react-router v5 exposes useHistory(); v6 exposes useNavigate(). Support both,
  // and fall back to a full reload if neither is present.
  const history = ReactRouterDOM.useHistory?.();
  const navigate = ReactRouterDOM.useNavigate?.();
  const go = () => {
    const path = `/plugins/awards/${performerId}`;
    if (history) history.push(path);
    else if (navigate) navigate(path);
    else window.location.assign(path);
  };
  return (
    <Button
      variant="secondary"
      title="Awards"
      className="awards-nav-button"
      style={{ marginRight: 8 }}
      onClick={go}
    >
      🏆 Awards
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

// after("PerformerPage") hands this (props, renderedResult).
export function injectAwardsButton(props: any, result: any): any {
  try {
    const performerId = getPerformerId(props);
    if (!performerId || !result) return result;
    return React.createElement(
      React.Fragment,
      null,
      React.createElement(AwardsNavButton, { key: "awards-button", performerId }),
      result
    );
  } catch (err) {
    console.warn("stash-awards: could not add the awards button", err);
    return result;
  }
}
