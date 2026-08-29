// The "Awards" entry button on the performer page.
//
// Earlier versions used `patch.after("PerformerPage", ...)`, but the patch
// point either does not exist or returns a non-element result on the Stash
// versions users actually run, which produced React #31 and a missing button.
// The DOM-injection approach below sidesteps Stash's React tree entirely:
// on every URL change, when the path matches /performers/:id, look for the
// performer navbar's action row and prepend a plain HTML button that
// navigates with history.pushState so react-router picks it up.

const NAV_BUTTON_CLASS = "awards-nav-button";

// findNavbar locates the Stash element we want to inject next to. Stash's
// performer page renders a row of icon buttons (Edit, Refresh, etc.) that
// sits at the top of the page; the row's child <button> elements are the
// most stable thing across versions because they are part of Stash's own
// navbar component, not a CSS class that the theme can rename.
//
// We try several well-known containers in order from most-specific to most-
// generic, and return the first one that contains at least one <button>.
function findNavbar(): HTMLElement | null {
  const candidates: (HTMLElement | null)[] = [
    document.querySelector(".performer-navbar"),
    document.querySelector(".performer-details .navbar"),
    document.querySelector(".navbar-buttons"),
    document.querySelector("[class*='performer'] [class*='navbar']"),
    document.querySelector("[class*='Performer'] [class*='Navbar']"),
  ];
  for (const c of candidates) {
    if (c && c.querySelector("button")) return c;
  }
  // Last resort: the first visible row of buttons on the page. This is the
  // least stable match and depends on Stash's structure not changing much.
  const allButtons = Array.from(document.querySelectorAll("button"));
  for (const b of allButtons) {
    const parent = b.closest("div, nav, header");
    if (!parent) continue;
    const rect = parent.getBoundingClientRect();
    if (rect.top > 200) continue; // only top-of-page rows
    if (parent.querySelectorAll("button").length >= 2) return parent as HTMLElement;
  }
  return null;
}

// navigate pushes the awards route the way react-router expects, then falls
// back to a hard navigation if the runtime context does not cooperate.
function navigate(path: string): void {
  if (window.history && window.history.pushState) {
    window.history.pushState({}, "", path);
    window.dispatchEvent(new PopStateEvent("popstate"));
  } else {
    window.location.assign(path);
  }
}

// buildButton returns the actual DOM node we drop into the navbar. Keeping
// it as plain DOM (rather than React-rendered) means Stash's own re-renders
// will not delete it.
function buildButton(performerId: string): HTMLButtonElement {
  const existing = document.querySelector<HTMLButtonElement>(
    `button.${NAV_BUTTON_CLASS}`
  );
  if (existing) return existing;

  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = `btn btn-secondary ${NAV_BUTTON_CLASS}`;
  btn.title = "Awards";
  btn.style.marginRight = "8px";
  btn.textContent = "🏆 Awards";
  btn.addEventListener("click", () => navigate(`/plugins/awards/${performerId}`));
  return btn;
}

// performerIdFromUrl returns the :id segment if the current URL is a
// performer page, else null. The URL is the only thing stable across Stash
// versions, so we read it directly.
function performerIdFromUrl(): string | null {
  const m = /^\/performers\/([^/?#]+)/.exec(window.location.pathname);
  return m ? decodeURIComponent(m[1]) : null;
}

// ensureButton inserts (or refreshes) the Awards button when on a performer
// page, and removes it elsewhere. Runs on every mutation of the body so a
// navigation away from the page cleans up after itself.
function ensureButton(): void {
  const performerId = performerIdFromUrl();
  const existing = document.querySelector<HTMLButtonElement>(
    `button.${NAV_BUTTON_CLASS}`
  );

  if (!performerId) {
    if (existing) existing.remove();
    return;
  }

  const navbar = findNavbar();
  if (!navbar) {
    // Stash hasn't rendered the navbar yet; the next mutation will try again.
    return;
  }

  const btn = buildButton(performerId);
  if (btn.parentElement === navbar) return;
  navbar.insertBefore(btn, navbar.firstChild);
}

// watch starts a MutationObserver that runs ensureButton on every DOM change.
// React re-renders mutate the navbar's children, so a one-shot insertion is
// not enough; the observer also deduplicates by checking parentElement.
export function startAwardsButton(): void {
  if (!document.body) {
    // The script ran before the body existed; wait and try again.
    document.addEventListener("DOMContentLoaded", startAwardsButton, {
      once: true,
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

  // react-router fires popstate on every internal navigation; that is also
  // a reliable cue to re-run, even if the DOM has not yet changed.
  window.addEventListener("popstate", ensureButton);

  ensureButton();
}