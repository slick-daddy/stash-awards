// Entry point Stash runs when it loads plugin/awards.js.
import { register, patch } from "./plugin";
import { AwardsPage } from "./AwardsPage";
import { injectAwardsButton } from "./PerformerButton";

// The standalone awards page.
register.route("/plugins/awards/:performerId", AwardsPage);

// The entry point on every performer page.
patch.after("PerformerPage", injectAwardsButton);
