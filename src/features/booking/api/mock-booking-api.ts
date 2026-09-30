import {
  BookingApiError,
  type AvailabilitySlot,
  type BookingApi,
  type BookingOptions,
  type ConfirmedAppointment,
} from "./contracts";

export const mockBookingOptions: BookingOptions = {
  vehicles: [
    {
      id: "vehicle-atlas",
      customerId: "customer-river",
      label: "Atlas Crossover",
      registration: "DEMO 214",
    },
    {
      id: "vehicle-nova",
      customerId: "customer-river",
      label: "Nova Hatchback",
      registration: "DEMO 508",
    },
  ],
  dealerships: [
    {
      id: "centre-harbour",
      name: "Harbour Service Centre",
      address: "18 Seabird Avenue",
      timezone: "Europe/London",
    },
    {
      id: "centre-meadow",
      name: "Meadow Service Centre",
      address: "42 Orchard Road",
      timezone: "Europe/London",
    },
  ],
  serviceTypes: [
    {
      id: "service-maintenance",
      name: "Routine maintenance",
      description: "Fluids, filters and a complete safety inspection.",
      durationMinutes: 60,
    },
    {
      id: "service-brakes",
      name: "Brake inspection",
      description: "Brake system inspection and diagnostic report.",
      durationMinutes: 90,
    },
  ],
};

export const mockSlots: AvailabilitySlot[] = [
  { startAt: "2026-10-02T09:00:00+01:00", endAt: "2026-10-02T10:00:00+01:00" },
  { startAt: "2026-10-02T10:30:00+01:00", endAt: "2026-10-02T11:30:00+01:00" },
  { startAt: "2026-10-02T13:00:00+01:00", endAt: "2026-10-02T14:00:00+01:00" },
  { startAt: "2026-10-02T15:30:00+01:00", endAt: "2026-10-02T16:30:00+01:00" },
];

interface MockOverrides extends Partial<BookingApi> {
  conflictOnFirstConfirmation?: boolean;
}

export function mockBookingApi(overrides: MockOverrides = {}): BookingApi {
  let confirmationAttempts = 0;
  const appointments = new Map<string, ConfirmedAppointment>();

  const api: BookingApi = {
    async getAppointments(status) {
      return {
        appointments: [...appointments.values()].filter(
          (appointment) => !status || appointment.status === status,
        ),
      };
    },
    async getBookingOptions() {
      return mockBookingOptions;
    },
    async getAvailability() {
      return { slots: mockSlots };
    },
    async confirmAppointment(request) {
      confirmationAttempts += 1;
      if (overrides.conflictOnFirstConfirmation && confirmationAttempts === 1) {
        throw new BookingApiError(
          "RESOURCE_CONFLICT",
          "That time was just taken.",
        );
      }

      const vehicle = mockBookingOptions.vehicles.find(
        ({ id }) => id === request.vehicleId,
      );
      const dealership = mockBookingOptions.dealerships.find(
        ({ id }) => id === request.dealershipId,
      );
      const serviceType = mockBookingOptions.serviceTypes.find(
        ({ id }) => id === request.serviceTypeId,
      );
      const slot = mockSlots.find(({ startAt }) => startAt === request.startAt);
      if (!vehicle || !dealership || !serviceType || !slot) {
        throw new BookingApiError(
          "VALIDATION_ERROR",
          "The appointment details are invalid.",
        );
      }

      const appointment: ConfirmedAppointment = {
        id: "appointment-demo-001",
        status: "CONFIRMED",
        vehicle,
        dealership,
        serviceType,
        technician: { id: "technician-morgan", name: "Morgan Lee" },
        serviceBay: { id: "bay-2", name: "Bay 2" },
        startAt: slot.startAt,
        endAt: slot.endAt,
      };
      appointments.set(appointment.id, appointment);
      return appointment;
    },
    async getAppointment(appointmentId) {
      const appointment = appointments.get(appointmentId);
      if (!appointment) {
        throw new BookingApiError("NOT_FOUND", "Appointment not found.");
      }
      return appointment;
    },
  };

  return { ...api, ...overrides };
}
