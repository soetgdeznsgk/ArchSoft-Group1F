const { createServer } = require("node:http");
const path = require("node:path");

const { renderPage } = require("./src/views");
const { serveStatic } = require("./src/static");
const { routePage } = require("./src/routes");

const port = Number.parseInt(process.env.PORT || "3000", 10);
const publicRoot = path.join(__dirname, "public");
const viewsRoot = path.join(__dirname, "views");

const server = createServer(async (request, response) => {
  const url = new URL(request.url || "/", "http://localhost");

  if (request.method !== "GET" && request.method !== "HEAD") {
    response.writeHead(405, { Allow: "GET, HEAD" });
    response.end("Method Not Allowed");
    return;
  }

  if (url.pathname === "/healthz") {
    response.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("ok\n");
    return;
  }

  if (url.pathname === "/") {
    response.writeHead(302, { Location: "/menu/" });
    response.end();
    return;
  }

  const page = routePage(url.pathname);
  if (page) {
    try {
      const html = await renderPage(page, viewsRoot);
      response.writeHead(200, {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "no-cache",
      });
      response.end(request.method === "HEAD" ? undefined : html);
    } catch (error) {
      console.error("SSR page rendering failed:", error);
      response.writeHead(500, { "Content-Type": "text/plain; charset=utf-8" });
      response.end("Internal Server Error");
    }
    return;
  }

  const served = await serveStatic(
    url.pathname, publicRoot, response, request.method, request.headers["accept-encoding"] || ""
  );
  if (!served) {
    response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("Not Found");
  }
});

server.listen(port, "0.0.0.0", () => {
  console.log(`SSR presentation listening on ${port}`);
});
