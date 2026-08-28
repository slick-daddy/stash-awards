// Builds the plugin UI: one IIFE bundle Stash loads as a plain <script>, plus
// the stylesheet. Stash runs the script in its own page, so everything the code
// needs (React, Bootstrap, the Apollo client, ...) is taken from
// window.PluginApi at runtime and nothing is bundled except this plugin's own
// source.
import esbuild from "esbuild";
import { readFile, writeFile } from "node:fs/promises";

await esbuild.build({
  entryPoints: ["src/awards.ts"],
  bundle: true,
  format: "iife",
  target: ["es2019"],
  outfile: "../plugin/awards.js",
  sourcemap: false,
  logLevel: "info",
  banner: { js: "/* GENERATED -- do not edit. Source: ui/src/*, built via npm run build in ui/ */" },
});

// Copy CSS with generated header so the artifact nature is explicit.
const css = await readFile("src/awards.css", "utf8");
const header = "/* GENERATED -- do not edit. Source: ui/src/awards.css, built via npm run build in ui/ */\n";
await writeFile("../plugin/awards.css", header + css);
