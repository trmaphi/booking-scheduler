import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import AppointmentLoading from "./loading";
import { loadAppointment, renderAppointmentPage } from "./appointment-view";

const appointmentId = "40000000-0000-4000-8000-000000000001";
const appointment = {
  id: appointmentId,
  status: "CONFIRMED",
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

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("appointment page", () => {
  it("retrieves persisted state with no-store and renders all assignments", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => response(appointment));

    render(
      await renderAppointmentPage({
        appointmentId,
        apiBaseURL: "http://api:8080",
        fetchImpl,
      }),
    );

    expect(fetchImpl).toHaveBeenCalledTimes(1);
    const [, init] = fetchImpl.mock.calls[0] ?? [];
    expect(init).toMatchObject({ cache: "no-store" });
    expect(
      screen.getByRole("heading", { name: "Appointment confirmed" }),
    ).toBeVisible();
    expect(screen.getByText("CONFIRMED")).toBeVisible();
    expect(screen.getByText("Silver Hatchback")).toBeVisible();
    expect(screen.getByText("Routine Inspection")).toBeVisible();
    expect(screen.getByText("Riverside Service Centre")).toBeVisible();
    expect(screen.getByText("Taylor Morgan")).toBeVisible();
    expect(screen.getByText("Bay A")).toBeVisible();
    expect(screen.getByText(/09:30.*10:30/)).toBeVisible();
  });

  it("retrieves the appointment again for every direct render", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => response(appointment));

    await loadAppointment(appointmentId, "http://api:8080", fetchImpl);
    await loadAppointment(appointmentId, "http://api:8080", fetchImpl);

    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it("renders a specific not-found state", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(
      response(
        {
          code: "NOT_FOUND",
          message: "appointment not found",
          requestId: "request-1",
        },
        404,
      ),
    );

    render(
      await renderAppointmentPage({
        appointmentId,
        apiBaseURL: "http://api:8080",
        fetchImpl,
      }),
    );

    expect(
      screen.getByRole("heading", { name: "Appointment not found" }),
    ).toBeVisible();
  });

  it("renders a temporary-unavailable state without leaking API details", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response("postgres.internal:5432", { status: 500 }),
      );

    render(
      await renderAppointmentPage({
        appointmentId,
        apiBaseURL: "http://api:8080",
        fetchImpl,
      }),
    );

    expect(
      screen.getByRole("heading", { name: "Appointment unavailable" }),
    ).toBeVisible();
    expect(screen.queryByText(/postgres\.internal/)).not.toBeInTheDocument();
  });

  it("renders a distinct loading state", () => {
    render(<AppointmentLoading />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Loading your appointment",
    );
  });
});
