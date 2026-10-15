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

Deno.test("presale route is TRON-native and keeps checkout disabled until configuration", async () => {
  const source = await Deno.readTextFile(new URL("./main.ts", import.meta.url));
  assertMatch(source, /path===\"\/presale\"/);
  assertMatch(source, /TRON MAINNET · TRX NATIVE/);
  assertMatch(source, /Install TronLink to connect your TRON wallet/);
  assertMatch(source, /Purchases are disabled until the verified sale contract is configured/);
  assertMatch(source, /Sale not configured/);
  assertEquals(source.includes("Phantom"), false);
  assertEquals(source.includes("Solflare"), false);
});
