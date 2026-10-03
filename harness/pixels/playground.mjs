// playground.mjs checks the browser playground end to end, as a visitor
// would use it: it serves playground/dist, loads the page in headless
// Chromium at desktop and phone sizes, in both themes, and checks every
// example draws, a mistake is named by its line, and a share link opens
// the same diagram. Screenshots go to the directory given, if one is.
//
//	make playground && node harness/pixels/playground.mjs [shots-dir]
import { chromium } from "playwright";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dist = fileURLToPath(new URL("../../playground/dist/", import.meta.url));
const shots = process.argv[2];
const types = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm" };
const server = createServer(async (req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  try {
    const body = await readFile(join(dist, path === "/" ? "index.html" : path));
    res.writeHead(200, { "content-type": types[extname(path || ".html")] ?? "text/html" });
    res.end(body);
  } catch {
    res.writeHead(404).end();
  }
}).listen(0);
const base = `http://localhost:${server.address().port}/`;

const failures = [];
const check = (ok, what) => { if (!ok) failures.push(what); };
const browser = await chromium.launch();
const errors = [];

async function open(viewport, colorScheme, url = base) {
  const ctx = await browser.newContext({ viewport, colorScheme, permissions: ["clipboard-read", "clipboard-write"] });
  const page = await ctx.newPage();
  page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(url);
  await page.waitForSelector("#notes li", { state: "attached", timeout: 60000 });
  return page;
}
const screen = (page) => page.evaluate(() => {
  const b = term.buffer.active, out = [];
  for (let y = 0; y < b.length; y++) out.push(b.getLine(y).translateToString(true));
  return out.join("\n");
});
const shot = async (page, name) => { if (shots) await page.screenshot({ path: `${shots}/${name}.png`, fullPage: true }); };

// Every example draws, at desktop size.
const desk = await open({ width: 1400, height: 860 }, "dark");
const n = await desk.locator("#example option").count();
check(n >= 4, `only ${n} examples`);
for (let i = 0; i < n; i++) {
  await desk.selectOption("#example", String(i));
  await desk.waitForTimeout(300);
  const name = await desk.locator(`#example option[value="${i}"]`).textContent();
  const notes = await desk.textContent("#notes");
  const text = await screen(desk);
  console.log(`${name}: ${await desk.textContent("#size")} | ${notes}`);
  check(!(await desk.$("#notes li.error")), `${name}: ${notes}`);
  const state = await desk.getAttribute("#status", "data-state");
  check(state === (notes.startsWith("Every") ? "ok" : "problem"), `${name}: the status icon says ${state} for "${notes}"`);
  check(/[╭┏╔]/.test(text), `${name}: no boxes in the terminal`);
  await shot(desk, `desktop-dark-${i}`);
}

// Settings open in a dialog, apply as they change, and close with Esc;
// a dot on their icon shows some differ from the defaults.
await desk.selectOption("#example", "0");
await desk.waitForTimeout(200);
const across = await desk.textContent("#size");
check(await desk.isHidden("#changed"), "the settings dot shows with the defaults");
await desk.click("#settings");
check(await desk.isVisible("#options"), "the settings dialog does not open");
await desk.click('#orient input[value="down"]');
await desk.waitForTimeout(200);
await shot(desk, "desktop-settings");
await desk.keyboard.press("Escape");
check(await desk.isHidden("#options"), "Esc does not close the settings");
check(await desk.textContent("#size") !== across, "turning the flow does not redraw it");
check(await desk.isVisible("#changed"), "the settings dot does not show a changed setting");
await desk.click("#settings");
await desk.click('#orient input[value=""]');
await desk.click("#closeOptions");
check(await desk.isHidden("#options") && await desk.isHidden("#changed"), "the close button or the dot is wrong");

// A mistake is named by its line, and nothing is drawn.
await desk.fill("#src", "flowchart LR\n  a --> b\n  a ~~> c");
await desk.waitForTimeout(300);
check((await desk.textContent("#notes li.error")).startsWith("line 3:"), "the mistake is not named by its line");
check(await desk.getAttribute("#status", "data-state") === "problem", "the status icon does not show the mistake");
check((await screen(desk)).includes("line 3:"), "the terminal does not say what is wrong");
check(await desk.isHidden("#notes"), "the tooltip shows without a hover");
await desk.hover("#status");
check(await desk.isVisible("#notes"), "the tooltip does not show on hover");
await shot(desk, "desktop-mistake-tooltip");
await desk.mouse.move(5, 5);

// A share link opens the same diagram, in the other theme.
const source = "flowchart LR\n  x[Shared] -->|ok| y([Link])";
await desk.fill("#src", source);
await desk.click("#share");
await desk.waitForTimeout(300);
const shared = await open({ width: 1400, height: 860 }, "light", desk.url());
check(await shared.inputValue("#src") === source, "the share link does not open the same source");
check((await screen(shared)).includes("[ ok ]"), "the shared diagram is not drawn");
check(await shared.inputValue("#example") === "", "the example menu does not say the diagram came from a link");
await shot(shared, "desktop-light-shared");

// The theme switch flips page and terminal, and is remembered.
const themed = await open({ width: 1400, height: 860 }, "light");
const termBg = () => themed.evaluate(() => term.options.theme.background);
const lightBg = await termBg();
await themed.click("#theme");
check(await themed.evaluate(() => document.documentElement.dataset.theme) === "dark", "the switch does not pick dark");
check(await termBg() !== lightBg, "the terminal does not follow the switch");
await shot(themed, "desktop-switched-dark");
await themed.reload();
await themed.waitForSelector("#notes li", { state: "attached" });
check(await themed.evaluate(() => document.documentElement.classList.contains("dark")), "the picked theme is not remembered");

// A phone gets the whole page, no wider than its screen, in both themes.
for (const scheme of ["light", "dark"]) {
  const phone = await open({ width: 390, height: 844 }, scheme);
  check(!(await phone.evaluate(() => document.documentElement.scrollWidth > innerWidth)), `${scheme} phone: the page scrolls sideways`);
  check((await phone.textContent("#size")).includes("fits"), `${scheme} phone: the agent loop does not fit`);
  await shot(phone, `phone-${scheme}`);
}

check(errors.length === 0, `console errors: ${errors.join("; ")}`);
await browser.close();
server.close();
if (failures.length) {
  console.error("\nFAILED:\n- " + failures.join("\n- "));
  process.exit(1);
}
console.log("\nThe playground works.");
