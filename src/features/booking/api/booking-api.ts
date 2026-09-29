import { createClient } from "./generated/client";
import type { ApiError } from "./generated/types.gen";
import {
  confirmAppointment,
  getAppointment,
  getAvailability,
  getBookingOptions,
} from "./generated/sdk.gen";
import {
  BookingApiError,
  type BookingApi,
  type BookingErrorCode,
} from "./contracts";

const errorCodes = new Set<BookingErrorCode>([
  "RESOURCE_CONFLICT",
  "IDEMPOTENCY_CONFLICT",
  "VALIDATION_ERROR",
  "NOT_FOUND",
  "INTERNAL_ERROR",
  "NOT_READY",
  "METHOD_NOT_ALLOWED",
]);

function normalizeBaseURL(baseURL: string) {
  const value = baseURL.trim().replace(/\/+$/, "");
  if (!value) {
    throw new Error(
      "Booking API base URL is missing. Configure NEXT_PUBLIC_API_BASE_URL for the browser or API_INTERNAL_BASE_URL for server rendering.",
    );
  }
  return value;
}

function mapError(reason: unknown): never {
  if (
    typeof reason === "object" &&
    reason !== null &&
    "name" in reason &&
    (reason.name === "AbortError" || reason.name === "TimeoutError")
  ) {
    throw reason;
  }

  if (
    typeof reason === "object" &&
    reason !== null &&
    "code" in reason &&
    "message" in reason
  ) {
    const error = reason as Partial<ApiError>;
    if (
      typeof error.code === "string" &&
      errorCodes.has(error.code as BookingErrorCode) &&
      typeof error.message === "string"
    ) {
      throw new BookingApiError(error.code as BookingErrorCode, error.message);
    }
  }

  throw new BookingApiError(
    "INTERNAL_ERROR",
    "The booking service is temporarily unavailable.",
  );
}

function mapRequestError(reason: unknown, signal?: AbortSignal): never {
  if (signal?.aborted) {
    throw new DOMException("The request was aborted.", "AbortError");
  }
  return mapError(reason);
}

export function createBookingApi(
  baseURL: string,
  fetchImpl: typeof fetch = globalThis.fetch,
): BookingApi {
  const client = createClient({
    baseUrl: normalizeBaseURL(baseURL),
    fetch: fetchImpl,
    throwOnError: true,
  });

  return {
    async getBookingOptions(signal) {
      try {
        const result = await getBookingOptions({
          client,
          signal,
          throwOnError: true,
        });
        return result.data;
      } catch (reason) {
        return mapRequestError(reason, signal);
      }
    },
    async getAvailability(request, signal) {
      try {
        const result = await getAvailability({
          client,
          query: request,
          signal,
          throwOnError: true,
        });
        return result.data;
      } catch (reason) {
        return mapRequestError(reason, signal);
      }
    },
    async confirmAppointment(request, signal) {
      const { idempotencyKey } = request;
      const body = {
        vehicleId: request.vehicleId,
        dealershipId: request.dealershipId,
        serviceTypeId: request.serviceTypeId,
        startAt: request.startAt,
      };
      try {
        const result = await confirmAppointment({
          client,
          body,
          headers: { "Idempotency-Key": idempotencyKey },
          signal,
          throwOnError: true,
        });
        return result.data;
      } catch (reason) {
        return mapRequestError(reason, signal);
      }
    },
    async getAppointment(appointmentId, signal) {
      try {
        const result = await getAppointment({
          client,
          path: { appointmentId },
          signal,
          throwOnError: true,
        });
        return result.data;
      } catch (reason) {
        return mapRequestError(reason, signal);
      }
    },
  };
}
