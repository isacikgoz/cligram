// The playground draws here, off the page's thread: laying out a big
// diagram takes longer than a frame, and the page keeps responding while
// it does. A request is {seq, request}; the answer {seq, result}, the
// drawing as JSON, or {seq, error} if cligram could not load.
importScripts("wasm_exec.js");

const loaded = (async () => {
  const go = new Go();
  const wasm = await WebAssembly.instantiateStreaming(fetch("cligram.wasm"), go.importObject);
  go.run(wasm.instance);
  while (typeof self.cligramDraw !== "function") await new Promise((r) => setTimeout(r, 5));
})();

onmessage = async (e) => {
  try {
    await loaded;
    postMessage({ seq: e.data.seq, result: self.cligramDraw(e.data.request) });
  } catch (err) {
    postMessage({ seq: e.data.seq, error: String(err && err.message || err) });
  }
};
