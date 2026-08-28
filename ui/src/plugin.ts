// Typed access to the runtime environment Stash provides. Stash injects
// window.PluginApi before any plugin script runs, and everything the UI needs
// comes from it — the plugin bundles none of it.
/* eslint-disable @typescript-eslint/no-explicit-any */
type Any = any;

const api: Any = (window as Any).PluginApi;

if (!api) {
  throw new Error(
    "stash-awards: window.PluginApi is missing; this script must be loaded by Stash"
  );
}

export const React = api.React as typeof import("react");
export const Apollo: Any = api.Apollo;
export const StashService: Any = api.utils.StashService;
export const register: {
  route: (path: string, component: Any) => void;
} = api.register;
export const patch: {
  before: (name: string, fn: Any) => void;
  after: (name: string, fn: Any) => void;
  instead: (name: string, fn: Any) => void;
} = api.patch;

// react-bootstrap is only loosely typed here; the components this UI uses take
// simple string props and typing them against stash's exact version would add
// matching @types packages for no real gain.
export const Bootstrap: Any = api.libraries.Bootstrap;
export const ReactRouterDOM: Any = api.libraries.ReactRouterDOM;
export const FontAwesomeIcon: Any =
  api.libraries.ReactFontAwesome.FontAwesomeIcon;
export const FontAwesomeSolid: Any = api.libraries.FontAwesomeSolid;
export const useToast: () => {
  success: (message: Any) => void;
  error: (message: Any) => void;
} = api.hooks.useToast;
