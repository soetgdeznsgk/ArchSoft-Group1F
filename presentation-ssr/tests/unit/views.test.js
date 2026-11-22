import { describe, expect, it } from "vitest";
import views from "../../src/views";

const { dataScript, rankingHtml, escapeHtml } = views;

describe("SSR ranking helpers", () => {
  it("renders a populated ranking", () => {
    const html = rankingHtml({
      players: [{ position: 1, nickname: "Ana", rating: 1016, games: 2, wins: 1 }],
    });
    expect(html).toContain("Ranking · Clásico");
    expect(html).toContain("<td>1016</td>");
  });

  it("renders empty and unavailable states", () => {
    expect(rankingHtml({ players: [] })).toContain("Aún no hay rankings disponibles.");
    expect(rankingHtml(null)).toContain("El ranking no está disponible por ahora.");
  });

  it("escapes names before inserting HTML", () => {
    expect(escapeHtml("<Ana>&")).toBe("&lt;Ana&gt;&amp;");
  });

  it("escapes closing script tags in SSR JSON", () => {
    const script = dataScript({ leaderboard: { players: [{ nickname: "</script>" }] } });
    const payload = script.slice(script.indexOf(">") + 1, script.lastIndexOf("<"));
    expect(payload).not.toContain("</script>");
    expect(payload).toContain("\\u003c/script>");
  });
});
