/**
 * RESTClient — Authenticated HTTP API calls.
 *
 * Wraps fetch with automatic JWT injection via AuthManager.
 * All non-upload methods set Content-Type: application/json.
 * Non-2xx responses throw ConstellError (401 → AuthError specifically).
 */

import type { AuthManager } from "./auth.js";
import { AuthError, ConstellError, NetworkError } from "./errors.js";

export class RESTClient {
  constructor(
    private auth: AuthManager,
    private baseUrl: string,
  ) {}

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  /** Perform an authenticated GET request. */
  async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  /** Perform an authenticated POST request with a JSON body. */
  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  /** Perform an authenticated PATCH request with a JSON body. */
  async patch<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("PATCH", path, body);
  }

  /** Perform an authenticated DELETE request. */
  async delete<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  /**
   * Upload files via multipart/form-data.
   * Does NOT set Content-Type so the browser can include the multipart boundary.
   */
  async upload<T>(path: string, formData: FormData): Promise<T> {
    const token = await this.auth.getValidToken();

    let response: Response;
    try {
      response = await fetch(`${this.baseUrl}${path}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
        },
        body: formData,
      });
    } catch (err) {
      throw new NetworkError(
        `Network error during ${path}: ${err instanceof Error ? err.message : String(err)}`,
      );
    }

    return this.handleResponse<T>(response);
  }

  // -------------------------------------------------------------------------
  // Private helpers
  // -------------------------------------------------------------------------

  /** Core request method shared by get / post / patch / delete. */
  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const token = await this.auth.getValidToken();

    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
    };

    const hasBody = body !== undefined;
    if (hasBody) {
      headers["Content-Type"] = "application/json";
    }

    let response: Response;
    try {
      response = await fetch(`${this.baseUrl}${path}`, {
        method,
        headers,
        body: hasBody ? JSON.stringify(body) : undefined,
      });
    } catch (err) {
      throw new NetworkError(
        `Network error during ${path}: ${err instanceof Error ? err.message : String(err)}`,
      );
    }

    return this.handleResponse<T>(response);
  }

  /**
   * Validate the response and parse JSON.
   * - 401 → AuthError
   * - other non-2xx → ConstellError
   */
  private async handleResponse<T>(response: Response): Promise<T> {
    if (response.ok) {
      return (await response.json()) as T;
    }

    // Attempt to extract an error message from the body
    let message = `HTTP ${response.status}`;
    try {
      const json = (await response.json()) as {
        message?: string;
        error?: string;
      };
      message = json.message ?? json.error ?? message;
    } catch {
      // use default message
    }

    if (response.status === 401) {
      throw new AuthError(message, 401);
    }

    throw new ConstellError("HTTP_ERROR", message, response.status);
  }
}
