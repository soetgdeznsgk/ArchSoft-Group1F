const { readFile } = require("node:fs/promises");
const path = require("node:path");

const VIEW_FILES = {
  menu: "menu.html",
  ingame: "ingame.html",
  chameleon: "chameleon.html",
};

async function renderPage(page, viewsRoot) {
  const fileName = VIEW_FILES[page];
  if (!fileName) throw new Error(`Unknown SSR page: ${page}`);
  return readFile(path.join(viewsRoot, fileName), "utf8");
}

module.exports = { renderPage };
