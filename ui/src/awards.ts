// Entry point Stash runs when it loads plugin/awards.js.
import { register } from "./plugin";
import { AwardsPage } from "./AwardsPage";
import { startAwardsButton } from "./PerformerButton";

// The standalone awards page.
register.route("/plugins/awards/:performerId", AwardsPage);

// The entry point on every performer page. Done by injecting a real DOM
// button into Stash's navbar rather than patching its React tree, because
// the patch point that previously worked ("PerformerPage") was renamed or
// returns a non-element result in current Stash releases.
startAwardsButton();
