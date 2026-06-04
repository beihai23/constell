/**
 * AuthManager — JWT token lifecycle management.
 *
 * Handles login, register, logout, token storage, and automatic refresh.
 * Tokens are persisted via a Storage interface (defaults to localStorage
 * in browser environments, no-op in Node).
 */

import type { User } from "./types.js";
import { AuthError, NetworkError } from "./errors.js";

// ---------------------------------------------------------------------------
// Storage abstraction
// ---------------------------------------------------------------------------

/**
 * Minimal subset of the Web Storage API used by AuthManager.
 * In Node / test environments, provide a Map-backed shim.
 */
export interface Storage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** No-op storage for environments without localStorage (e.g. Node). */
const noopStorage: Storage = {
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
};

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ACCESS_TOKEN_KEY = "constell_access_token";
const REFRESH_TOKEN_KEY = "constell_refresh_token";

/** Refresh this many seconds before actual expiry. */
const REFRESH_BUFFER_SEC = 60;

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

interface JWTPayload {
  sub?: string;
  exp?: number;
  [key: string]: unknown;
}

/** Decode the payload section of a JWT without validation. */
function decodeJWTPayload(token: string): JWTPayload {
  const parts = token.split(".");
  if (parts.length < 2) {
    throw new AuthError("Invalid JWT format");
  }
  // index 0 = header, index 1 = payload
  const b64 = parts[1];
  // base64url → base64
  const padded = b64.replace(/-/g, "+").replace(/_/g, "/");
  const json = decodeURIComponent(
    atob(padded)
      .split("")
      .map((c) => "%" + ("00" + c.charCodeAt(0).toString(16)).slice(-2))
      .join(""),
  );
  return JSON.parse(json) as JWTPayload;
}

// ---------------------------------------------------------------------------
// API response shapes (mirrors backend auth handlers)
// ---------------------------------------------------------------------------

interface AuthResponse {
  user: User;
  access_token: string;
  refresh_token: string;
}

interface RefreshResponse {
  access_token: string;
  refresh_token: string;
}

// ---------------------------------------------------------------------------
// AuthManager
// ---------------------------------------------------------------------------

export class AuthManager {
  private readonly apiUrl: string;
  private readonly storage: Storage;

  /** Singleton promise so concurrent callers share one refresh request. */
  private refreshPromise: Promise<string> | null = null;

  constructor(apiUrl: string, storage?: Storage) {
    this.apiUrl = apiUrl;
    this.storage = storage ??
      (typeof globalThis !== "undefined" && "localStorage" in globalThis
        ? (globalThis as unknown as { localStorage: Storage }).localStorage
        : noopStorage);
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  /**
   * Log in with email and password.
   * On success the tokens are stored and the authenticated user is returned.
   */
  async login(email: string, password: string): Promise<User> {
    const res = await this.doFetch<AuthResponse>("/api/v1/auth/login", {
      email,
      password,
    });
    this.storeTokens(res.access_token, res.refresh_token);
    return res.user;
  }

  /**
   * Register a new account.
   * On success the tokens are stored and the new user is returned.
   */
  async register(username: string, email: string, password: string): Promise<User> {
    const res = await this.doFetch<AuthResponse>("/api/v1/auth/register", {
      username,
      email,
      password,
    });
    this.storeTokens(res.access_token, res.refresh_token);
    return res.user;
  }

  /** Clear stored tokens, effectively logging out. */
  logout(): void {
    this.storage.removeItem(ACCESS_TOKEN_KEY);
    this.storage.removeItem(REFRESH_TOKEN_KEY);
  }

  /** Return `true` if a stored access token exists and is not expired. */
  isAuthenticated(): boolean {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!token) return false;
    try {
      const payload = decodeJWTPayload(token);
      if (!payload.exp) return false;
      // Still valid if > now + buffer
      return payload.exp > Date.now() / 1000 + REFRESH_BUFFER_SEC;
    } catch {
      return false;
    }
  }

  /**
   * Return a valid access token, refreshing transparently when expired.
   *
   * Multiple concurrent callers share a single in-flight refresh via the
   * `refreshPromise` singleton.
   */
  async getValidToken(): Promise<string> {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!token) {
      throw new AuthError("No access token available — please log in");
    }

    // Check expiry
    try {
      const payload = decodeJWTPayload(token);
      const exp = payload.exp ?? 0;
      const now = Date.now() / 1000;

      if (exp > now + REFRESH_BUFFER_SEC) {
        // Token still valid — return it
        return token;
      }
    } catch {
      // Malformed token — fall through to refresh
    }

    // Token expired or malformed — refresh
    return this.refresh();
  }

  /**
   * Parse the stored access token to reconstruct a User object.
   * Returns `null` when no token is stored or the payload is invalid.
   */
  initFromStorage(): User | null {
    const token = this.storage.getItem(ACCESS_TOKEN_KEY);
    if (!token) return null;

    try {
      const payload = decodeJWTPayload(token);
      if (!payload.sub) return null;

      return {
        id: payload.sub,
        email: (payload.email as string) ?? "",
        nickname: (payload.nickname as string) ?? "",
        avatarUrl: (payload.avatar_url as string) ?? (payload.avatarUrl as string) ?? "",
        statusMessage: (payload.status_message as string) ?? (payload.statusMessage as string) ?? "",
        createdAt: ((payload.created_at ?? payload.createdAt) as number) ?? 0,
        updatedAt: ((payload.updated_at ?? payload.updatedAt) as number) ?? 0,
      };
    } catch {
      return null;
    }
  }

  // -------------------------------------------------------------------------
  // Private helpers
  // -------------------------------------------------------------------------

  /** Store both tokens in storage. */
  private storeTokens(accessToken: string, refreshToken: string): void {
    this.storage.setItem(ACCESS_TOKEN_KEY, accessToken);
    this.storage.setItem(REFRESH_TOKEN_KEY, refreshToken);
  }

  /**
   * Refresh the access token using the stored refresh token.
   * Uses a singleton promise so concurrent callers share one request.
   */
  private refresh(): Promise<string> {
    if (this.refreshPromise) {
      return this.refreshPromise;
    }

    this.refreshPromise = (async () => {
      try {
        const refreshToken = this.storage.getItem(REFRESH_TOKEN_KEY);
        if (!refreshToken) {
          throw new AuthError("No refresh token available — please log in");
        }

        const res = await this.doFetch<RefreshResponse>("/api/v1/auth/refresh", {
          refresh_token: refreshToken,
        });

        this.storeTokens(res.access_token, res.refresh_token);
        return res.access_token;
      } catch (err) {
        // Refresh failed — clear tokens so caller knows to re-authenticate
        this.logout();
        throw err;
      } finally {
        this.refreshPromise = null;
      }
    })();

    return this.refreshPromise;
  }

  /**
   * Perform a JSON POST to the API gateway.
   * Throws AuthError on 4xx, NetworkError on fetch failures.
   */
  private async doFetch<T>(path: string, body: Record<string, unknown>): Promise<T> {
    let response: Response;
    try {
      response = await fetch(`${this.apiUrl}${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
    } catch (err) {
      throw new NetworkError(
        `Network error during ${path}: ${err instanceof Error ? err.message : String(err)}`,
      );
    }

    if (!response.ok) {
      let message = `HTTP ${response.status}`;
      try {
        const json = (await response.json()) as { message?: string; error?: string };
        message = json.message ?? json.error ?? message;
      } catch {
        // use default message
      }
      throw new AuthError(message, response.status);
    }

    return (await response.json()) as T;
  }
}
