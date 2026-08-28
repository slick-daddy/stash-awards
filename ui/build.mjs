// Builds the plugin UI: one IIFE bundle Stash loads as a plain <script>, plus
// the stylesheet. Stash runs the script in its own page, so everything the code
// needs (React, Bootstrap, the Apollo client, ...) is taken from
// window.PluginApi at runtime and nothing is bundled except this plugin's own
// source.
import esbuild from "esbuild";
import { copyFile } from "node:fs/promises";

await esbuild.build({
  entryPoints: ["src/awards.ts"],
  bundle: true,
  format: "iife",
  target: ["es2019"],
  outfile: "../plugin/awards.js",
  sourcemap: false,
  logLevel: "info",
});

await copyFile("src/awards.css", "../plugin/awards.css");
