const PAGE_ROUTES = new Map([
  ["/menu/", "menu"],
  ["/ingame/", "ingame"],
  ["/chameleon/", "chameleon"],
]);

function routePage(pathname) {
  return PAGE_ROUTES.get(pathname) || null;
}

module.exports = { routePage };
