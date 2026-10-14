import { assertEquals, assertMatch } from "jsr:@std/assert@1";
Deno.test("manifest metadata has TRON brand and app icon", async () => {
  const text = await Deno.readTextFile(new URL("./manifest.webmanifest", import.meta.url));
  const manifest = JSON.parse(text);
  assertMatch(manifest.name, /TRON Console/);
  assertEquals(manifest.theme_color, "#111318");
  assertEquals(manifest.icons[0].src, "/icon.svg");
});
Deno.test("SVG icon uses TRON red and dark background", async () => {
  const icon = await Deno.readTextFile(new URL("./icon.svg", import.meta.url));
  assertMatch(icon, /#eb0029/);
  assertMatch(icon, /#111318/);
});

Deno.test("console is locked to TRON mainnet and has no testnet node endpoints", async () => {
  const source = await Deno.readTextFile(new URL("./main.ts", import.meta.url));
  assertMatch(source, /const NETWORK = "mainnet"/);
  assertMatch(source, /const NODE = "https:\/\/api\.trongrid\.io"/);
  assertEquals(source.includes("nile.trongrid.io"), false);
  assertEquals(source.includes("api.shasta.trongrid.io"), false);
  assertEquals(source.includes("TRON_NETWORK"), false);
  assertMatch(source, /hostname\(\)==="api\.trongrid\.io"/);
});
