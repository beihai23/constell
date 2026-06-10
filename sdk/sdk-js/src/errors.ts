/**
 * Base error class for all Constell SDK errors.
 *
 * Every SDK-thrown error is a subclass of {@link ConstellError}, making it
 * easy to distinguish backend/SDK errors from unrelated runtime exceptions.
 */
export class ConstellError extends Error {
  /** Machine-readable error code returned by the backend (e.g. "UNAUTHORIZED"). */
  readonly code: string;
  /** HTTP status code (0 when not applicable, e.g. WebSocket errors). */
  readonly statusCode: number;

  constructor(code: string, message: string, statusCode = 0) {
    super(message);
    this.name = "ConstellError";
    this.code = code;
    this.statusCode = statusCode;
  }
}

/**
 * Thrown when an authentication or authorisation request fails
 * (invalid credentials, expired token, etc.).
 */
export class AuthError extends ConstellError {
  constructor(message: string, statusCode = 401) {
    super("AUTH_ERROR", message, statusCode);
    this.name = "AuthError";
  }
}

/**
 * Thrown when a network-level failure occurs (fetch error, WebSocket
 * disconnect, timeout, DNS failure, etc.).
 */
export class NetworkError extends ConstellError {
  constructor(message: string) {
    super("NETWORK_ERROR", message, 0);
    this.name = "NetworkError";
  }
}
