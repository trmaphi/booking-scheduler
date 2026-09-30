import { describe, expect, it, vi } from "vitest";

import { BookingApiError } from "./contracts";
import { createBookingApi } from "./booking-api";

const appointment = {
  id: "40000000-0000-4000-8000-000000000001",
  status: "CONFIRMED" as const,
  vehicle: {
    id: "20000000-0000-0000-0000-000000000011",
    customerId: "20000000-0000-0000-0000-000000000001",
    label: "Silver Hatchback",
    registration: "DEMO-001",
  },
  dealership: {
    id: "20000000-0000-0000-0000-000000000021",
    name: "Riverside Service Centre",
    address: "100 Riverside Way",
    timezone: "Europe/London",
  },
  serviceType: {
    id: "20000000-0000-0000-0000-000000000041",
    name: "Routine Inspection",
    description: "Routine inspection",
    durationMinutes: 60,
  },
  technician: {
    id: "20000000-0000-0000-0000-000000000051",
    name: "Taylor Morgan",
  },
  serviceBay: {
    id: "20000000-0000-0000-0000-000000000061",
    name: "Bay A",
  },
  startAt: "2031-03-04T09:30:00Z",
  endAt: "2031-03-04T10:30:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("createBookingApi", () => {
  it("encodes availability queries with the generated operation", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({
        slots: [
          {
            startAt: "2031-03-04T09:30:00Z",
            endAt: "2031-03-04T10:30:00Z",
          },
        ],
      }),
    );
    const api = createBookingApi("http://localhost:8080/", fetchImpl);

    await api.getAvailability({
      vehicleId: appointment.vehicle.id,
      dealershipId: appointment.dealership.id,
      serviceTypeId: appointment.serviceType.id,
      date: "2031-03-04",
    });

    const request = fetchImpl.mock.calls[0]?.[0] as Request;
    expect(request.url).toBe(
      "http://localhost:8080/api/v1/availability?vehicleId=20000000-0000-0000-0000-000000000011&dealershipId=20000000-0000-0000-0000-000000000021&serviceTypeId=20000000-0000-0000-0000-000000000041&date=2031-03-04",
    );
  });

  it.each([201, 200])(
    "returns a confirmed appointment from status %s",
    async (status) => {
      const fetchImpl = vi
        .fn<typeof fetch>()
        .mockResolvedValue(jsonResponse(appointment, status));
      const api = createBookingApi("http://localhost:8080", fetchImpl);

      await expect(
        api.confirmAppointment({
          vehicleId: appointment.vehicle.id,
          dealershipId: appointment.dealership.id,
          serviceTypeId: appointment.serviceType.id,
          date: "2031-03-04",
          startAt: appointment.startAt,
          idempotencyKey: "booking-request-1234",
        }),
      ).resolves.toEqual(appointment);

      const request = fetchImpl.mock.calls[0]?.[0] as Request;
      expect(request.headers.get("Idempotency-Key")).toBe(
        "booking-request-1234",
      );
      expect(await request.clone().json()).toEqual({
        vehicleId: appointment.vehicle.id,
        dealershipId: appointment.dealership.id,
        serviceTypeId: appointment.serviceType.id,
        startAt: appointment.startAt,
      });
    },
  );

  it("preserves API error codes", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse(
        {
          code: "RESOURCE_CONFLICT",
          message: "the requested appointment is no longer available",
          requestId: "request-1",
        },
        409,
      ),
    );

    await expect(
      createBookingApi("http://localhost:8080", fetchImpl).getBookingOptions(),
    ).rejects.toMatchObject({
      name: "BookingApiError",
      code: "RESOURCE_CONFLICT",
    });
  });

  it("uses a safe error when an error response is malformed", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response("database address leaked", { status: 500 }),
      );

    await expect(
      createBookingApi("http://localhost:8080", fetchImpl).getBookingOptions(),
    ).rejects.toEqual(
      new BookingApiError(
        "INTERNAL_ERROR",
        "The booking service is temporarily unavailable.",
      ),
    );
  });

  it("propagates cancellation to fetch", async () => {
    const controller = new AbortController();
    const fetchImpl = vi.fn<typeof fetch>((request) => {
      const signal = (request as Request).signal;
      return new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => {
          reject(new DOMException("Aborted", "AbortError"));
        });
      });
    });

    const pending = createBookingApi(
      "http://localhost:8080",
      fetchImpl,
    ).getBookingOptions(controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
  });

  it("retrieves an appointment through the generated path operation", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(jsonResponse(appointment));

    await expect(
      createBookingApi("http://localhost:8080", fetchImpl).getAppointment(
        appointment.id,
      ),
    ).resolves.toEqual(appointment);

    const request = fetchImpl.mock.calls[0]?.[0] as Request;
    expect(request.url).toBe(
      `http://localhost:8080/api/v1/appointments/${appointment.id}`,
    );
  });

  it("lists appointments with an optional status filter", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(jsonResponse({ appointments: [appointment] }));

    await expect(
      createBookingApi("http://localhost:8080", fetchImpl).getAppointments(
        "CONFIRMED",
      ),
    ).resolves.toEqual({ appointments: [appointment] });

    const request = fetchImpl.mock.calls[0]?.[0] as Request;
    expect(request.url).toBe(
      "http://localhost:8080/api/v1/appointments?status=CONFIRMED",
    );
  });
});
