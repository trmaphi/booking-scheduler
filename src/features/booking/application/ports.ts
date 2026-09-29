import type { AppointmentInterval, ResourceAssignment } from "../domain/models";

export interface AvailabilityRepository {
  findAvailableIntervals(input: {
    dealershipId: string;
    serviceTypeId: string;
    date: string;
  }): Promise<AppointmentInterval[]>;
}

export interface AppointmentRepository {
  confirm(input: {
    customerId: string;
    vehicleId: string;
    dealershipId: string;
    serviceTypeId: string;
    startAt: string;
    idempotencyKey: string;
  }): Promise<
    ResourceAssignment & AppointmentInterval & { appointmentId: string }
  >;
}
