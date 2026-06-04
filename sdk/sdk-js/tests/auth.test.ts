import { describe, it, expect, vi, beforeEach } from "vitest";
import { AuthManager } from "../src/auth.js";
import type { Storage } from "../src/auth.js";
import { AuthError, NetworkError } from "../src/errors.js";
import type { User } from "../src/types.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Map-backed Storage shim for Node test environment. */
function createStorage(): Storage {
  const map = new Map<string, string>();
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => { map.set(key, value); },
    removeItem: (key: string) => { map.delete(key); },
  };
}

/**
 * Create a fake JWT with the given payload.
 * Only the payload (index 1) is real base64; header and signature are dummy.
 */
function fakeJWT(payload: Record<string, unknown>): string {
  const json = JSON.stringify(payload);
  const b64 = btoa(json)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `eyJhbGciOiJIUzI1NiJ9.${b64}.fakesignature`;
}

const API_URL = "http://localhost:8080";

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("AuthManager", () => {
  let storage: Storage;
  let auth: AuthManager;

  beforeEach(() => {
    storage = createStorage();
    auth = new AuthManager(API_URL, storage);
    vi.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // login
  // -------------------------------------------------------------------------
  describe("login", () => {
    it("stores tokens and returns user on successful login", async () => {
      const accessToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
      const refreshToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 86400 });
      const user: User = {
        id: "u1",
        email: "test@example.com",
        nickname: "tester",
        avatarUrl: "",
        statusMessage: "",
        createdAt: 1000,
        updatedAt: 1000,
      };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ user, access_token: accessToken, refresh_token: refreshToken }),
      } as Response);

      const result = await auth.login("test@example.com", "password123");

      expect(result).toEqual(user);
      expect(storage.getItem("constell_access_token")).toBe(accessToken);
      expect(storage.getItem("constell_refresh_token")).toBe(refreshToken);
    });

    it("throws AuthError on invalid credentials", async () => {
      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ message: "invalid credentials" }),
      } as Response);

      await expect(auth.login("bad@example.com", "wrong")).rejects.toThrow(AuthError);
    });

    it("throws NetworkError on fetch failure", async () => {
      vi.spyOn(globalThis, "fetch").mockRejectedValueOnce(new TypeError("Failed to fetch"));

      await expect(auth.login("a@b.com", "pw")).rejects.toThrow(NetworkError);
    });
  });

  // -------------------------------------------------------------------------
  // register
  // -------------------------------------------------------------------------
  describe("register", () => {
    it("stores tokens and returns user on successful registration", async () => {
      const accessToken = fakeJWT({ sub: "u2", exp: Date.now() / 1000 + 3600 });
      const refreshToken = fakeJWT({ sub: "u2", exp: Date.now() / 1000 + 86400 });
      const user: User = {
        id: "u2",
        email: "new@example.com",
        nickname: "newuser",
        avatarUrl: "",
        statusMessage: "",
        createdAt: 2000,
        updatedAt: 2000,
      };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ user, access_token: accessToken, refresh_token: refreshToken }),
      } as Response);

      const result = await auth.register("newuser", "new@example.com", "password123");

      expect(result).toEqual(user);
      expect(storage.getItem("constell_access_token")).toBe(accessToken);
      expect(storage.getItem("constell_refresh_token")).toBe(refreshToken);
    });
  });

  // -------------------------------------------------------------------------
  // logout
  // -------------------------------------------------------------------------
  describe("logout", () => {
    it("clears stored tokens", () => {
      storage.setItem("constell_access_token", "some-token");
      storage.setItem("constell_refresh_token", "some-refresh");

      auth.logout();

      expect(storage.getItem("constell_access_token")).toBeNull();
      expect(storage.getItem("constell_refresh_token")).toBeNull();
    });
  });

  // -------------------------------------------------------------------------
  // getValidToken
  // -------------------------------------------------------------------------
  describe("getValidToken", () => {
    it("returns stored token when not expired", async () => {
      const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
      storage.setItem("constell_access_token", token);

      const result = await auth.getValidToken();
      expect(result).toBe(token);
    });

    it("throws AuthError when no token is stored", async () => {
      await expect(auth.getValidToken()).rejects.toThrow(AuthError);
    });

    it("refreshes when token is expired", async () => {
      // Token expired 100 seconds ago
      const expiredToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 - 100 });
      storage.setItem("constell_access_token", expiredToken);
      storage.setItem("constell_refresh_token", "old-refresh");

      const newAccessToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
      const newRefreshToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 86400 });

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          access_token: newAccessToken,
          refresh_token: newRefreshToken,
        }),
      } as Response);

      const result = await auth.getValidToken();

      expect(result).toBe(newAccessToken);
      expect(storage.getItem("constell_access_token")).toBe(newAccessToken);
      expect(storage.getItem("constell_refresh_token")).toBe(newRefreshToken);

      // Verify the refresh request body
      const fetchCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      const body = JSON.parse(fetchCall[1].body as string);
      expect(body.refresh_token).toBe("old-refresh");
    });

    it("clears tokens and throws when refresh fails", async () => {
      const expiredToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 - 100 });
      storage.setItem("constell_access_token", expiredToken);
      storage.setItem("constell_refresh_token", "bad-refresh");

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ message: "refresh token expired" }),
      } as Response);

      await expect(auth.getValidToken()).rejects.toThrow(AuthError);

      // Tokens should be cleared (logout)
      expect(storage.getItem("constell_access_token")).toBeNull();
      expect(storage.getItem("constell_refresh_token")).toBeNull();
    });

    it("shares a single refresh request among concurrent callers", async () => {
      const expiredToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 - 100 });
      storage.setItem("constell_access_token", expiredToken);
      storage.setItem("constell_refresh_token", "some-refresh");

      const newAccessToken = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });

      // Delay the response to ensure concurrent callers hit the same promise
      let resolveResponse: (value: unknown) => void;
      const responsePromise = new Promise((resolve) => {
        resolveResponse = resolve;
      });

      vi.spyOn(globalThis, "fetch").mockImplementationOnce(async () => {
        await responsePromise;
        return {
          ok: true,
          status: 200,
          json: async () => ({
            access_token: newAccessToken,
            refresh_token: "new-refresh",
          }),
        } as Response;
      });

      // Fire 3 concurrent getValidToken calls
      const p1 = auth.getValidToken();
      const p2 = auth.getValidToken();
      const p3 = auth.getValidToken();

      // Resolve the refresh response
      resolveResponse!(undefined);

      const [t1, t2, t3] = await Promise.all([p1, p2, p3]);

      // All three should get the same token
      expect(t1).toBe(newAccessToken);
      expect(t2).toBe(newAccessToken);
      expect(t3).toBe(newAccessToken);

      // Only one fetch call should have been made
      expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    });
  });

  // -------------------------------------------------------------------------
  // isAuthenticated
  // -------------------------------------------------------------------------
  describe("isAuthenticated", () => {
    it("returns true when token is present and not expired", () => {
      const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
      storage.setItem("constell_access_token", token);

      expect(auth.isAuthenticated()).toBe(true);
    });

    it("returns false when no token is stored", () => {
      expect(auth.isAuthenticated()).toBe(false);
    });

    it("returns false when token is expired", () => {
      const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 - 100 });
      storage.setItem("constell_access_token", token);

      expect(auth.isAuthenticated()).toBe(false);
    });

    it("returns false when token is within 60-second refresh buffer", () => {
      // Token expires in 30 seconds — within the 60-second buffer
      const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 30 });
      storage.setItem("constell_access_token", token);

      expect(auth.isAuthenticated()).toBe(false);
    });
  });

  // -------------------------------------------------------------------------
  // initFromStorage
  // -------------------------------------------------------------------------
  describe("initFromStorage", () => {
    it("restores user from valid token", () => {
      const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
      storage.setItem("constell_access_token", token);

      const user = auth.initFromStorage();

      expect(user).not.toBeNull();
      expect(user!.id).toBe("u1");
    });

    it("returns null when no token is stored", () => {
      expect(auth.initFromStorage()).toBeNull();
    });

    it("returns null for token without sub claim", () => {
      const token = fakeJWT({ exp: Date.now() / 1000 + 3600 });
      storage.setItem("constell_access_token", token);

      expect(auth.initFromStorage()).toBeNull();
    });

    it("returns null for malformed token", () => {
      storage.setItem("constell_access_token", "not-a-jwt");

      expect(auth.initFromStorage()).toBeNull();
    });
  });
});
