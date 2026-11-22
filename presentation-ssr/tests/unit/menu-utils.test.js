import { describe, expect, it } from "vitest";
import utils from "../../../presentation/lobby/menu-utils";

describe("lobby menu utilities", () => {
  it("normalizes guest names", () => {
    expect(utils.normalizeGuestName("  Ana María  ")).toBe("Ana María");
  });

  it("normalizes room codes", () => {
    expect(utils.normalizeRoomCode("  ab12  ")).toBe("AB12");
  });

  it("accepts valid account credentials", () => {
    expect(utils.validateCredentials("  ana_1 ", "safe-pass-123")).toEqual({
      ok: true,
      username: "ana_1",
    });
  });

  it("rejects invalid credentials", () => {
    expect(utils.validateCredentials("a!", "safe-pass").ok).toBe(false);
    expect(utils.validateCredentials("ana", "short").ok).toBe(false);
  });
});
