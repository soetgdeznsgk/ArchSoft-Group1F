import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import serverModule from "../../server";

const { createPresentationServer } = serverModule;

let server;
let baseUrl;
let tempRoot;

beforeEach(async () => {
  tempRoot = await fs.mkdtemp(path.join(os.tmpdir(), "bololo-presentation-"));
  await fs.mkdir(path.join(tempRoot, "views"), { recursive: true });
  await fs.mkdir(path.join(tempRoot, "public", "menu"), { recursive: true });
  await fs.writeFile(
    path.join(tempRoot, "views", "menu.html"),
    "<main><!--ssr:ranking--><!--ssr:data--></main>"
  );
  await fs.writeFile(path.join(tempRoot, "views", "ingame.html"), "<main>Mesa</main>");
  await fs.writeFile(path.join(tempRoot, "views", "chameleon.html"), "<main>Camaleón</main>");
  await fs.writeFile(path.join(tempRoot, "public", "menu", "menu.js"), "window.testFrontend = true;");

  server = createPresentationServer({
    publicRootOverride: path.join(tempRoot, "public"),
    viewsRootOverride: path.join(tempRoot, "views"),
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  baseUrl = `http://127.0.0.1:${address.port}`;
});

afterEach(async () => {
  await new Promise((resolve) => server.close(resolve));
  await fs.rm(tempRoot, { recursive: true, force: true });
});

describe("presentation SSR server", () => {
  it("redirects root and serves health", async () => {
    const root = await fetch(`${baseUrl}/`, { redirect: "manual" });
    expect(root.status).toBe(302);
    expect(root.headers.get("location")).toBe("/menu/");
    expect(await (await fetch(`${baseUrl}/healthz`)).text()).toBe("ok\n");
  });

  it("serves all page templates", async () => {
    expect(await (await fetch(`${baseUrl}/menu/`)).text()).toContain("ssr-data");
    expect(await (await fetch(`${baseUrl}/ingame/`)).text()).toBe("<main>Mesa</main>");
    expect(await (await fetch(`${baseUrl}/chameleon/`)).text()).toBe("<main>Camaleón</main>");
  });

  it("serves static assets and rejects unknown paths", async () => {
    const asset = await fetch(`${baseUrl}/menu/menu.js`);
    expect(asset.status).toBe(200);
    expect(asset.headers.get("content-type")).toContain("javascript");
    expect((await fetch(`${baseUrl}/missing`)).status).toBe(404);
  });
});
