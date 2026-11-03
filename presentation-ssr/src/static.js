const { stat } = require("node:fs/promises");
const { createReadStream } = require("node:fs");
const path = require("node:path");
const { createGzip } = require("node:zlib");

const COMPRESSIBLE = new Set([".css", ".html", ".js", ".svg"]);

const CONTENT_TYPES = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".jpg": "image/jpeg",
  ".mp3": "audio/mpeg",
  ".png": "image/png",
  ".svg": "image/svg+xml",
};

async function serveStatic(pathname, publicRoot, response, method, acceptEncoding = "") {
  if (!pathname.startsWith("/libraries/") &&
      !pathname.startsWith("/menu/") &&
      !pathname.startsWith("/ingame/") &&
      !pathname.startsWith("/chameleon/")) {
    return false;
  }

  const relativePath = pathname.replace(/^\/+/, "");
  const filePath = path.resolve(publicRoot, relativePath);
  if (!filePath.startsWith(`${path.resolve(publicRoot)}${path.sep}`)) return false;

  let fileInfo;
  try {
    fileInfo = await stat(filePath);
  } catch {
    return false;
  }
  if (!fileInfo.isFile()) return false;

  const extension = path.extname(filePath).toLowerCase();
  const contentType = CONTENT_TYPES[extension] || "application/octet-stream";
  const gzip = COMPRESSIBLE.has(extension) && /\bgzip\b/.test(acceptEncoding);
  const headers = { "Content-Type": contentType, "Cache-Control": "no-cache", Vary: "Accept-Encoding" };
  if (gzip) headers["Content-Encoding"] = "gzip";
  response.writeHead(200, headers);
  if (method === "HEAD") {
    response.end();
    return true;
  }
  const stream = createReadStream(filePath);
  if (gzip) stream.pipe(createGzip()).pipe(response);
  else stream.pipe(response);
  return true;
}

module.exports = { serveStatic };
