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

  /**
   * Upload files via multipart/form-data with per-upload progress.
   * Uses XHR (fetch can't report request-body progress). The optional
   * onProgress callback receives a 0..1 fraction as the request body uploads.
   */
  uploadWithProgress<T>(
    path: string,
    formData: FormData,
    onProgress?: (fraction: number) => void,
  ): Promise<T> {
    // getValidToken is async; wrap the whole thing so the public signature can
    // stay a plain Promise<T> (callers don't want to await the token first).
    return this.auth.getValidToken().then(
      (token) =>
        new Promise<T>((resolve, reject) => {
          const xhr = new XMLHttpRequest();
          xhr.open("POST", `${this.baseUrl}${path}`);
          xhr.setRequestHeader("Authorization", `Bearer ${token}`);
          if (onProgress) {
            xhr.upload.onprogress = (e) => {
              if (e.lengthComputable) onProgress(e.loaded / e.total);
            };
          }
          xhr.onload = () => {
            if (xhr.status >= 200 && xhr.status < 300) {
              try {
                resolve(JSON.parse(xhr.responseText) as T);
              } catch (err) {
                reject(
                  new ConstellError(
                    "HTTP_ERROR",
                    `Invalid JSON in upload response: ${err instanceof Error ? err.message : String(err)}`,
                  ),
                );
              }
            } else {
              let message = `HTTP ${xhr.status}`;
              try {
                const json = JSON.parse(xhr.responseText) as {
                  message?: string;
                  error?: string;
                };
                message = json.message ?? json.error ?? message;
              } catch {
                // use default
              }
              if (xhr.status === 401) reject(new AuthError(message, 401));
              else reject(new ConstellError("HTTP_ERROR", message, xhr.status));
            }
          };
          xhr.onerror = () =>
            reject(new NetworkError(`Network error during ${path}`));
          xhr.send(formData);
        }),
    );
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
