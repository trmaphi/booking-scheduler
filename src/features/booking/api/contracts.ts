export type Identifier = string;

export interface VehicleOption {
  id: Identifier;
  customerId: Identifier;
  label: string;
  registration: string;
}

export interface DealershipOption {
  id: Identifier;
  name: string;
  address: string;
  timezone: string;
}

export interface ServiceTypeOption {
  id: Identifier;
  name: string;
  description: string;
  durationMinutes: number;
}

export interface BookingOptions {
  vehicles: VehicleOption[];
  dealerships: DealershipOption[];
  serviceTypes: ServiceTypeOption[];
}

export interface AvailabilityRequest {
  vehicleId: Identifier;
  dealershipId: Identifier;
  serviceTypeId: Identifier;
  date: string;
}

export interface AvailabilitySlot {
  startAt: string;
  endAt: string;
}

export interface AvailabilityResult {
  slots: AvailabilitySlot[];
}

export interface ConfirmAppointmentRequest extends AvailabilityRequest {
  startAt: string;
  idempotencyKey: string;
}

export interface ConfirmedAppointment {
  id: Identifier;
  status: "CONFIRMED" | "CANCELLED";
  vehicle: VehicleOption;
  dealership: DealershipOption;
  serviceType: ServiceTypeOption;
  technician: { id: Identifier; name: string };
  serviceBay: { id: Identifier; name: string };
  startAt: string;
  endAt: string;
}

export interface AppointmentListResult {
  appointments: ConfirmedAppointment[];
}

export type BookingErrorCode =
  | "RESOURCE_CONFLICT"
  | "IDEMPOTENCY_CONFLICT"
  | "VALIDATION_ERROR"
  | "NOT_FOUND"
  | "INTERNAL_ERROR"
  | "NOT_READY"
  | "METHOD_NOT_ALLOWED";

export class BookingApiError extends Error {
  constructor(
    public readonly code: BookingErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "BookingApiError";
  }
}

export interface BookingApi {
  getAppointments(
    status?: ConfirmedAppointment["status"],
    signal?: AbortSignal,
  ): Promise<AppointmentListResult>;
  getBookingOptions(signal?: AbortSignal): Promise<BookingOptions>;
  getAvailability(
    request: AvailabilityRequest,
    signal?: AbortSignal,
  ): Promise<AvailabilityResult>;
  confirmAppointment(
    request: ConfirmAppointmentRequest,
    signal?: AbortSignal,
  ): Promise<ConfirmedAppointment>;
  getAppointment(
    appointmentId: Identifier,
    signal?: AbortSignal,
  ): Promise<ConfirmedAppointment>;
}
