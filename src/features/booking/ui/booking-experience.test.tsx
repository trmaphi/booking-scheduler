import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { BookingApi, BookingOptions } from "../api/contracts";
import {
  mockBookingApi,
  mockBookingOptions,
  mockSlots,
} from "../api/mock-booking-api";
import { BookingExperience } from "./booking-experience";

const navigation = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
}));

async function completeSearch(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole("option", { name: /Atlas Crossover/i });
  await user.selectOptions(screen.getByLabelText("Vehicle"), "vehicle-atlas");
  await user.selectOptions(
    screen.getByLabelText("Service centre"),
    "centre-harbour",
  );
  await user.selectOptions(
    screen.getByLabelText("Service"),
    "service-maintenance",
  );
  await user.type(screen.getByLabelText("Preferred date"), "2026-10-02");
  await user.click(
    screen.getByRole("button", { name: "Find available times" }),
  );
}

describe("BookingExperience", () => {
  beforeEach(() => navigation.push.mockReset());

  it("shows an actionable message when the browser API URL is missing", () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "");
    render(<BookingExperience />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "NEXT_PUBLIC_API_BASE_URL",
    );
    vi.unstubAllEnvs();
  });
  it("guides a keyboard user from booking details to a review", async () => {
    const user = userEvent.setup();
    render(<BookingExperience api={mockBookingApi()} />);

    await screen.findByRole("option", { name: /Atlas Crossover/i });
    expect(
      screen.getByRole("button", { name: "Find available times" }),
    ).toBeDisabled();
    await completeSearch(user);

    const slot = await screen.findByRole("radio", { name: /09:00/ });
    await user.click(slot);
    await user.click(
      screen.getByRole("button", { name: "Review appointment" }),
    );

    expect(
      screen.getByRole("heading", { name: "Review your appointment" }),
    ).toBeVisible();
    expect(screen.getByText("Routine maintenance · 60 minutes")).toBeVisible();
    expect(
      screen.getByText(
        /technician and service bay are assigned when you confirm/i,
      ),
    ).toBeVisible();
  });

  it("formats the selected date and interval in the service centre timezone", async () => {
    const user = userEvent.setup();
    const options = {
      ...mockBookingOptions,
      dealerships: mockBookingOptions.dealerships.map((dealership) =>
        dealership.id === "centre-harbour"
          ? { ...dealership, timezone: "Pacific/Auckland" }
          : dealership,
      ),
    };
    const api = mockBookingApi({
      getBookingOptions: vi.fn().mockResolvedValue(options),
      getAvailability: vi.fn().mockResolvedValue({
        slots: [
          {
            startAt: "2026-10-02T11:30:00Z",
            endAt: "2026-10-02T12:30:00Z",
          },
        ],
      }),
    });
    render(<BookingExperience api={api} />);

    await screen.findByRole("option", { name: /Atlas Crossover/i });
    await user.selectOptions(screen.getByLabelText("Vehicle"), "vehicle-atlas");
    await user.selectOptions(
      screen.getByLabelText("Service centre"),
      "centre-harbour",
    );
    await user.selectOptions(
      screen.getByLabelText("Service"),
      "service-maintenance",
    );
    await user.type(screen.getByLabelText("Preferred date"), "2026-10-03");
    await user.click(
      screen.getByRole("button", { name: "Find available times" }),
    );

    expect(
      await screen.findByRole("heading", { name: "Saturday, 3 October 2026" }),
    ).toBeVisible();
    await user.click(screen.getByRole("radio", { name: /00:30/ }));
    await user.click(
      screen.getByRole("button", { name: "Review appointment" }),
    );
    expect(screen.getByText("00:30–01:30")).toBeVisible();
  });

  it("falls back safely when a service centre timezone is invalid", async () => {
    const user = userEvent.setup();
    const options = {
      ...mockBookingOptions,
      dealerships: mockBookingOptions.dealerships.map((dealership) =>
        dealership.id === "centre-harbour"
          ? { ...dealership, timezone: "Not/A_Timezone" }
          : dealership,
      ),
    };
    const api = mockBookingApi({
      getBookingOptions: vi.fn().mockResolvedValue(options),
      getAvailability: vi.fn().mockResolvedValue({
        slots: [
          {
            startAt: "2026-10-02T11:30:00Z",
            endAt: "2026-10-02T12:30:00Z",
          },
        ],
      }),
    });
    render(<BookingExperience api={api} />);

    await screen.findByRole("option", { name: /Atlas Crossover/i });
    await user.selectOptions(screen.getByLabelText("Vehicle"), "vehicle-atlas");
    await user.selectOptions(
      screen.getByLabelText("Service centre"),
      "centre-harbour",
    );
    await user.selectOptions(
      screen.getByLabelText("Service"),
      "service-maintenance",
    );
    await user.type(screen.getByLabelText("Preferred date"), "2026-10-02");
    await user.click(
      screen.getByRole("button", { name: "Find available times" }),
    );

    expect(
      await screen.findByRole("radio", { name: /11:30/ }),
    ).toBeInTheDocument();
  });

  it("announces loading and empty availability states", async () => {
    let resolveOptions:
      ((value: typeof mockBookingOptions) => void) | undefined;
    const loadingApi: BookingApi = mockBookingApi({
      getBookingOptions: vi.fn(
        (): Promise<BookingOptions> =>
          new Promise((resolve) => {
            resolveOptions = resolve;
          }),
      ),
      getAvailability: vi.fn().mockResolvedValue({ slots: [] }),
    });

    const user = userEvent.setup();
    render(<BookingExperience api={loadingApi} />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Preparing your booking",
    );

    resolveOptions?.(mockBookingOptions);
    await completeSearch(user);

    expect(await screen.findByText(/No times are available/)).toBeVisible();
  });

  it("preserves valid selections and refreshes times after a stale-slot conflict", async () => {
    const user = userEvent.setup();
    const api = mockBookingApi({ conflictOnFirstConfirmation: true });
    api.getAvailability = vi.fn(api.getAvailability);
    render(<BookingExperience api={api} />);

    await completeSearch(user);
    await user.click(await screen.findByRole("radio", { name: /09:00/ }));
    await user.click(
      screen.getByRole("button", { name: "Review appointment" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Confirm appointment" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /time was just taken/i,
    );
    expect(screen.getByLabelText("Vehicle")).toHaveValue("vehicle-atlas");
    expect(screen.getByLabelText("Service centre")).toHaveValue(
      "centre-harbour",
    );
    expect(screen.getByLabelText("Service")).toHaveValue("service-maintenance");
    expect(api.getAvailability).toHaveBeenCalledTimes(2);
    expect(screen.getByText("Choose another time")).toBeVisible();
  });

  it("leaves review immediately and shows a truthful error when conflict refresh fails", async () => {
    const user = userEvent.setup();
    const api = mockBookingApi({ conflictOnFirstConfirmation: true });
    api.getAvailability = vi
      .fn()
      .mockResolvedValueOnce({ slots: mockSlots })
      .mockRejectedValueOnce(new Error("network unavailable"));
    render(<BookingExperience api={api} />);

    await completeSearch(user);
    await user.click(await screen.findByRole("radio", { name: /09:00/ }));
    await user.click(
      screen.getByRole("button", { name: "Review appointment" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Confirm appointment" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /could not refresh availability/i,
    );
    expect(screen.getByText("Choose another time")).toBeVisible();
    expect(screen.getByLabelText("Vehicle")).toHaveValue("vehicle-atlas");
    expect(screen.getByLabelText("Service centre")).toHaveValue(
      "centre-harbour",
    );
    expect(screen.getByLabelText("Service")).toHaveValue("service-maintenance");
    expect(screen.getByLabelText("Preferred date")).toHaveValue("2026-10-02");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Review appointment" }),
    ).toBeDisabled();
  });

  it("navigates to the durable appointment URL after confirmation", async () => {
    const user = userEvent.setup();
    render(<BookingExperience api={mockBookingApi()} />);

    await completeSearch(user);
    await user.click(await screen.findByRole("radio", { name: /09:00/ }));
    await user.click(
      screen.getByRole("button", { name: "Review appointment" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Confirm appointment" }),
    );

    await waitFor(() =>
      expect(navigation.push).toHaveBeenCalledWith(
        "/appointments/appointment-demo-001",
      ),
    );
  });
});
