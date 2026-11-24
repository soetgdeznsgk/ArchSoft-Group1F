const BololoMenuUtils = {
  normalizeGuestName(name) {
    return String(name || "").trim();
  },

  normalizeUsername(username) {
    return String(username || "").trim();
  },

  validateCredentials(username, password) {
    const normalizedUsername = this.normalizeUsername(username);
    if (!/^[A-Za-z0-9_.-]+$/.test(normalizedUsername)) {
      return { ok: false, error: "USERNAMé debe estar escrito usando letras, números, punto, guion o guion bajo." };
    }
    if (normalizedUsername.length < 3 || normalizedUsername.length > 20) {
      return { ok: false, error: "USERNAMé debe tener entre 3 y 20 caracteres" };
    }
    const passwordBytes = new TextEncoder().encode(password || "").length;
    if (passwordBytes < 8) return { ok: false, error: "La contraseña debe tener al menos 8 caracteres" };
    if (passwordBytes > 72) return { ok: false, error: "La contraseña es demasiado larga" };
    return { ok: true, username: normalizedUsername };
  },

  normalizeRoomCode(code) {
    return String(code || "").trim().toUpperCase();
  },
};

if (typeof module !== "undefined" && module.exports) module.exports = BololoMenuUtils;
