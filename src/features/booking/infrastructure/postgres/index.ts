import type {
  AppointmentRepository,
  AvailabilityRepository,
} from "../../application/ports";

export interface BookingPostgresAdapters {
  appointments: AppointmentRepository;
  availability: AvailabilityRepository;
}
