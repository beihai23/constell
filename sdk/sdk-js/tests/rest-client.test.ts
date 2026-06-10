import { describe, it, expect, vi, beforeEach } from "vitest";
import { AuthManager } from "../src/auth.js";
import type { Storage } from "../src/auth.js";
import { RESTClient } from "../src/rest-client.js";
import { AuthError, ConstellError } from "../src/errors.js";

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

describe("RESTClient", () => {
  let storage: Storage;
  let auth: AuthManager;
  let client: RESTClient;

  beforeEach(() => {
    storage = createStorage();
    auth = new AuthManager(API_URL, storage);

    // Pre-store a valid (non-expired) JWT so getValidToken() succeeds
    const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
    storage.setItem("constell_access_token", token);
    storage.setItem("constell_refresh_token", fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 86400 }));

    client = new RESTClient(auth, API_URL);
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // GET
  // ---------------------------------------------------------------------------
  describe("get", () => {
    it("sends GET request with Authorization header", async () => {
      const token = storage.getItem("constell_access_token")!;
      const expectedData = { id: "ch1", name: "general" };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => expectedData,
      } as Response);

      const result = await client.get<{ id: string; name: string }>("/api/v1/channels/ch1");

      expect(result).toEqual(expectedData);
      expect(globalThis.fetch).toHaveBeenCalledTimes(1);

      const [url, init] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toBe(`${API_URL}/api/v1/channels/ch1`);
      expect(init.method).toBe("GET");
      expect(init.headers).toEqual({
        Authorization: `Bearer ${token}`,
      });
      // GET should not send a body
      expect(init.body).toBeUndefined();
    });
  });

  // ---------------------------------------------------------------------------
  // POST
  // ---------------------------------------------------------------------------
  describe("post", () => {
    it("sends POST request with JSON body and Content-Type header", async () => {
      const token = storage.getItem("constell_access_token")!;
      const requestBody = { content: "Hello world" };
      const responseData = { id: "m1", content: "Hello world" };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => responseData,
      } as Response);

      const result = await client.post<{ id: string; content: string }>(
        "/api/v1/channels/ch1/messages",
        requestBody,
      );

      expect(result).toEqual(responseData);

      const [url, init] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toBe(`${API_URL}/api/v1/channels/ch1/messages`);
      expect(init.method).toBe("POST");
      expect(init.headers).toEqual({
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      });
      expect(JSON.parse(init.body as string)).toEqual(requestBody);
    });
  });

  // ---------------------------------------------------------------------------
  // PATCH
  // ---------------------------------------------------------------------------
  describe("patch", () => {
    it("sends PATCH request with JSON body", async () => {
      const token = storage.getItem("constell_access_token")!;
      const requestBody = { content: "Updated message" };
      const responseData = { id: "m1", content: "Updated message" };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => responseData,
      } as Response);

      const result = await client.patch<{ id: string; content: string }>(
        "/api/v1/channels/ch1/messages/m1",
        requestBody,
      );

      expect(result).toEqual(responseData);

      const [url, init] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toBe(`${API_URL}/api/v1/channels/ch1/messages/m1`);
      expect(init.method).toBe("PATCH");
      expect(init.headers).toEqual({
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      });
      expect(JSON.parse(init.body as string)).toEqual(requestBody);
    });
  });

  // ---------------------------------------------------------------------------
  // Error handling
  // ---------------------------------------------------------------------------
  describe("error handling", () => {
    it("throws ConstellError on non-2xx response", async () => {
      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({ message: "internal server error" }),
      } as Response);

      try {
        await client.get("/api/v1/channels/ch1");
        expect.unreachable("Should have thrown");
      } catch (err) {
        expect(err).toBeInstanceOf(ConstellError);
        expect(err).not.toBeInstanceOf(AuthError);
        expect((err as ConstellError).statusCode).toBe(500);
        expect((err as ConstellError).message).toBe("internal server error");
      }
    });

    it("throws AuthError on 401 response", async () => {
      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ message: "unauthorized" }),
      } as Response);

      try {
        await client.get("/api/v1/channels/ch1");
        expect.unreachable("Should have thrown");
      } catch (err) {
        expect(err).toBeInstanceOf(AuthError);
        expect((err as AuthError).statusCode).toBe(401);
        expect((err as AuthError).message).toBe("unauthorized");
      }
    });
  });

  // ---------------------------------------------------------------------------
  // Upload
  // ---------------------------------------------------------------------------
  describe("upload", () => {
    it("sends FormData without Content-Type header", async () => {
      const token = storage.getItem("constell_access_token")!;
      const formData = new FormData();
      formData.append("file", new Blob(["hello"], { type: "text/plain" }), "test.txt");

      const responseData = { id: "f1", url: "http://minio:9000/files/test.txt" };

      vi.spyOn(globalThis, "fetch").mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => responseData,
      } as Response);

      const result = await client.upload<{ id: string; url: string }>(
        "/api/v1/files/upload",
        formData,
      );

      expect(result).toEqual(responseData);

      const [url, init] = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toBe(`${API_URL}/api/v1/files/upload`);
      expect(init.method).toBe("POST");
      // Only Authorization header — no Content-Type so browser sets multipart boundary
      expect(init.headers).toEqual({
        Authorization: `Bearer ${token}`,
      });
      expect(init.body).toBe(formData);
    });
  });
});
