const { stat } = require("node:fs/promises");
const { createReadStream } = require("node:fs");
const path = require("node:path");

const CONTENT_TYPES = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".jpg": "image/jpeg",
  ".mp3": "audio/mpeg",
  ".png": "image/png",
  ".svg": "image/svg+xml",
};

async function serveStatic(pathname, publicRoot, response, method) {
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

  const contentType = CONTENT_TYPES[path.extname(filePath).toLowerCase()] ||
    "application/octet-stream";
  response.writeHead(200, {
    "Content-Type": contentType,
    "Cache-Control": "no-cache",
  });
  if (method === "HEAD") {
    response.end();
    return true;
  }
  createReadStream(filePath).pipe(response);
  return true;
}

module.exports = { serveStatic };
