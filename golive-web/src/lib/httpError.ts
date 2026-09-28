import axios from 'axios';

/**
 * The HTTP status of a failed request, or undefined when no response came
 * back at all (offline, timeout, blocked).
 */
export function httpStatus(error: unknown): number | undefined {
  return axios.isAxiosError(error) ? error.response?.status : undefined;
}

/**
 * True only when the server answered that the thing doesn't exist. Any other
 * failure means we don't know, and must not be shown as "not found" or "ended".
 */
export function isNotFoundError(error: unknown): boolean {
  return httpStatus(error) === 404;
}
