import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/booking/ui/booking-experience", () => ({
  BookingExperience: () => null,
}));

import { renderAppointmentsPage } from "./appointments-view";
import AppointmentsLoading from "./loading";
import Home from "../page";

const appointment = {
  id: "40000000-0000-4000-8000-000000000001",
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

describe("appointments page", () => {
  it("renders a distinct loading state", () => {
    render(<AppointmentsLoading />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Loading appointments",
    );
  });

  it("is linked from the booking page navigation", () => {
    render(<Home />);
    expect(screen.getByRole("link", { name: "Appointments" })).toHaveAttribute(
      "href",
      "/appointments",
    );
  });

  it("renders booked schedules and status filters", async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(response({ appointments: [appointment] }));

    render(
      await renderAppointmentsPage({
        status: "CONFIRMED",
        apiBaseURL: "http://api:8080",
        fetchImpl,
      }),
    );

    expect(screen.getByRole("heading", { name: "Appointments" })).toBeVisible();
    expect(screen.getByRole("link", { name: "All" })).toHaveAttribute(
      "href",
      "/appointments",
    );
    expect(screen.getByRole("link", { name: "Confirmed" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Cancelled" })).toBeVisible();
    expect(screen.getByText("Silver Hatchback")).toBeVisible();
    expect(screen.getByText("Routine Inspection")).toBeVisible();
    expect(screen.getByText("Riverside Service Centre")).toBeVisible();
    expect(screen.getByText("Taylor Morgan · Bay A")).toBeVisible();
    expect(
      screen.getByRole("link", { name: /View appointment/ }),
    ).toHaveAttribute("href", `/appointments/${appointment.id}`);
    const request = fetchImpl.mock.calls[0]?.[0] as Request;
    expect(request.url).toBe(
      "http://api:8080/api/v1/appointments?status=CONFIRMED",
    );
  });
});
