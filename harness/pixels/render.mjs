// render.mjs draws cligram cases in a real terminal engine and saves what
// it shows, for the pixel checks in internal/pixels to look at.
//
//	node render.mjs [out]
//
// It reads out/cases/*.json (written by TestExportPixelCases), renders each
// with xterm.js in headless Chromium once per font, and writes, per font,
// out/shots/<font>/<case>.png with <case>.json giving where the cells are.
// It also renders an atlas: every glyph and color the cases use, each on
// its own with blank cells round it, which the checks compare cells with.
import { chromium } from "playwright";
import { readFileSync, writeFileSync, mkdirSync, readdirSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "node:http";
import { extname } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const out = resolve(process.argv[2] ?? join(here, "out"));

// The page, its scripts and its fonts come from a server on this machine:
// browsers load fonts only from an origin.
const types = { ".css": "text/css", ".js": "text/javascript", ".ttf": "font/ttf", ".woff2": "font/woff2", ".woff": "font/woff", ".html": "text/html" };
const server = createServer((req, res) => {
  const path = join(here, decodeURIComponent(new URL(req.url, "http://x").pathname));
  if (!path.startsWith(here)) return res.writeHead(403).end();
  try {
    res.writeHead(200, { "content-type": types[extname(path)] ?? "application/octet-stream" }).end(readFileSync(path));
  } catch {
    res.writeHead(404).end();
  }
});
await new Promise((done) => server.listen(0, "127.0.0.1", done));
const origin = `http://127.0.0.1:${server.address().port}`;
const url = (p) => `${origin}/${p}`;

// The fonts drawn with: a monospace font whole, then fallbacks for wide
// text and emoji, as a terminal would fall back. The emoji are Noto Emoji's
// outlines: headless Chromium will not take the color font's tables, and
// it is the cells an emoji takes that are checked, not its colors.
const fallback = "'Noto Sans JP', 'Noto Emoji'";
const fonts = {
  jetbrains: `'JetBrains Mono Pinned', ${fallback}`,
  dejavu: `'DejaVu Sans Mono Pinned', ${fallback}`,
};

// A fixed palette, so a color means the same in every run.
const theme = {
  background: "#000000", foreground: "#d0d0d0",
  black: "#000000", red: "#cd3131", green: "#0dbc79", yellow: "#e5e510",
  blue: "#2472c8", magenta: "#bc3fbc", cyan: "#11a8cd", white: "#e5e5e5",
  brightBlack: "#767676", brightRed: "#f14c4c", brightGreen: "#23d18b", brightYellow: "#f5f543",
  brightBlue: "#3b8eea", brightMagenta: "#d670d6", brightCyan: "#29b8db", brightWhite: "#ffffff",
};

const page = `<!doctype html>
<meta charset="utf-8">
<link rel="stylesheet" href="${url("node_modules/@xterm/xterm/css/xterm.css")}">
<link rel="stylesheet" href="${url("node_modules/@fontsource/noto-sans-jp/400.css")}">
<link rel="stylesheet" href="${url("node_modules/@fontsource/noto-emoji/400.css")}">
<style>
  @font-face { font-family: 'JetBrains Mono Pinned'; src: url(${url("fonts/JetBrainsMono-Regular.ttf")}); }
  @font-face { font-family: 'DejaVu Sans Mono Pinned'; src: url(${url("fonts/DejaVuSansMono.ttf")}); }
  body { margin: 0; background: #000; }
  #term { display: inline-block; }
</style>
<script src="${url("node_modules/@xterm/xterm/lib/xterm.js")}"></script>
<script src="${url("node_modules/@xterm/addon-unicode11/lib/addon-unicode11.js")}"></script>
<script src="${url("node_modules/@xterm/addon-webgl/lib/addon-webgl.js")}"></script>
<div id="term"></div>`;

// draw renders ansi in a cols by rows terminal in font, and gives where
// its cells are on the page.
async function draw(p, ansi, cols, rows, family) {
  return p.evaluate(async ({ ansi, cols, rows, family, theme }) => {
    const host = document.getElementById("term");
    host.innerHTML = "";
    const term = new Terminal({
      cols, rows, fontFamily: family, fontSize: 32, lineHeight: 1, letterSpacing: 0,
      allowProposedApi: true, theme, cursorBlink: false, disableStdin: true,
      drawBoldTextInBrightColors: false, scrollback: 0,
      // Box drawing from the font, as most terminals take it, not drawn
      // by xterm.js itself.
      customGlyphs: false,
    });
    term.loadAddon(new Unicode11Addon.Unicode11Addon());
    term.unicode.activeVersion = "11";
    term.open(host);
    // The WebGL renderer draws each glyph into its own cell, as GPU
    // terminals do; the DOM renderer lays text out as a browser does and
    // lets it drift from the grid along a row.
    const webgl = new WebglAddon.WebglAddon();
    term.loadAddon(webgl);
    // No cursor; lines end with a return as well as a newline.
    await new Promise((done) => term.write("\x1b[?25l" + ansi.replaceAll("\n", "\r\n"), done));
    await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)));
    const r = host.querySelector(".xterm-screen").getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height, cols, rows };
  }, { ansi, cols, rows, family, theme });
}

// atlas lays out every glyph and color the cases use, each alone with
// blank cells round it, and gives where each one is.
function atlas(cases) {
  const seen = new Map();
  for (const c of cases) {
    for (const row of c.cells) {
      (row ?? []).forEach((cell, x) => {
        if (cell.cont || cell.g === " " || cell.g === "") return;
        const key = `${cell.sgr ?? ""}\x00${cell.g}`;
        const w = row[x + 1]?.cont ? 2 : 1; // the cells it takes, as cligram measured
        if (!seen.has(key)) seen.set(key, { g: cell.g, sgr: cell.sgr ?? "", w });
      });
    }
  }
  // A character no font has, to show what a missing glyph looks like.
  seen.set("tofu", { g: "\u0378", sgr: "", w: 1, tofu: true });
  // Ten to a row, six cells apart, with a blank row between rows; each
  // placed by moving the cursor to its column, so wide glyphs shift nothing.
  const cols = 60, per = 10;
  const items = [...seen.values()];
  items.forEach((it, i) => {
    it.y = 1 + 2 * Math.floor(i / per);
    it.x = 2 + 6 * (i % per);
  });
  const rows = items.length ? items[items.length - 1].y + 2 : 1;
  const text = Array.from({ length: rows }, (_, y) =>
    items.filter((it) => it.y === y)
      .map((it) => `\x1b[${it.x + 1}G` + (it.sgr ? `\x1b[${it.sgr}m` : "") + it.g + "\x1b[0m")
      .join("")).join("\n");
  return { cols, rows, ansi: text, items };
}

const caseDir = join(out, "cases");
const cases = readdirSync(caseDir).filter((f) => f.endsWith(".json")).sort()
  .map((f) => ({ name: f.replace(/\.json$/, ""), ...JSON.parse(readFileSync(join(caseDir, f), "utf8")) }));
const at = atlas(cases);

// WebGL in a headless browser: in software, the same on every machine.
const browser = await chromium.launch({ args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"] });
try {
  // At scale 1 with a large font: the WebGL renderer draws a scaled page
  // wrongly in a headless browser, and a large font shows the same detail.
  const p = await browser.newPage({ deviceScaleFactor: 1, viewport: { width: 8000, height: 4000 } });
  writeFileSync(join(here, "out", "page.html"), page);
  await p.goto(url("out/page.html"), { waitUntil: "load" });
  // Every font loaded before the first terminal measures its cells.
  await p.evaluate(async () => {
    for (const f of ["32px 'JetBrains Mono Pinned'", "32px 'DejaVu Sans Mono Pinned'", "32px 'Noto Sans JP'", "32px 'Noto Emoji'"]) {
      await document.fonts.load(f, "─│╭╮╰╯═║╔╗┏┓━┃▸▴▾◂✓✗◔…審査検証🚀");
    }
    await document.fonts.ready;
  });
  for (const [font, family] of Object.entries(fonts)) {
    const dir = join(out, "shots", font);
    mkdirSync(dir, { recursive: true });
    const shoot = async (name, ansi, cols, rows) => {
      const where = await draw(p, ansi, cols, rows, family);
      await p.locator("#term .xterm-screen").screenshot({ path: join(dir, `${name}.png`) });
      writeFileSync(join(dir, `${name}.json`), JSON.stringify({ ...where, scale: 1 }));
    };
    await shoot("_atlas", at.ansi, at.cols, at.rows);
    writeFileSync(join(dir, "_atlas.items.json"), JSON.stringify(at.items));
    for (const c of cases) await shoot(c.name, c.ansi, c.cols, c.rows);
    console.log(`${font}: ${cases.length} cases and an atlas of ${at.items.length} glyphs`);
  }
} finally {
  await browser.close();
  server.close();
}
